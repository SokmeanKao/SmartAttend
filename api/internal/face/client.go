package face

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

var (
	ErrFaceServiceUnavailable = errors.New("face service unavailable")
	ErrFaceServiceTimeout     = errors.New("face service timeout")
)

type DetectedPose struct {
	Label    Pose    `json:"label"`
	YawScore float32 `json:"yaw_score"`
}

type EmbedResult struct {
	QualityScore float32
	Pose         DetectedPose
	Template     StagedTemplate
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *Client) Embed(
	ctx context.Context,
	image []byte,
	filename string,
	contentType string,
	expectedPose Pose,
) (EmbedResult, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", filename)
	if err != nil {
		return EmbedResult{}, err
	}
	if _, err := part.Write(image); err != nil {
		return EmbedResult{}, err
	}
	if err := writer.WriteField("expected_pose", string(expectedPose)); err != nil {
		return EmbedResult{}, err
	}
	if err := writer.Close(); err != nil {
		return EmbedResult{}, err
	}

	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"/internal/v1/faces/embed", &body,
	)
	if err != nil {
		return EmbedResult{}, err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if contentType != "" {
		request.Header.Set("X-Image-Content-Type", contentType)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return EmbedResult{}, ErrFaceServiceTimeout
		}
		return EmbedResult{}, fmt.Errorf("%w: %v", ErrFaceServiceUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, response.Body)
		return EmbedResult{}, fmt.Errorf("%w: status %d", ErrFaceServiceUnavailable, response.StatusCode)
	}

	var payload struct {
		QualityScore      float32      `json:"quality_score"`
		Pose              DetectedPose `json:"pose"`
		Embedding         string       `json:"embedding"`
		EmbeddingEncoding string       `json:"embedding_encoding"`
		EmbeddingDim      int          `json:"embedding_dim"`
		ModelName         string       `json:"model_name"`
		ModelVersion      string       `json:"model_version"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&payload); err != nil {
		return EmbedResult{}, fmt.Errorf("%w: invalid response", ErrFaceServiceUnavailable)
	}
	embedding, err := base64.StdEncoding.DecodeString(payload.Embedding)
	if err != nil {
		return EmbedResult{}, fmt.Errorf("invalid embedding encoding: %w", err)
	}
	if payload.EmbeddingEncoding != "float32-le-base64" ||
		payload.EmbeddingDim <= 0 ||
		len(embedding) != payload.EmbeddingDim*4 {
		return EmbedResult{}, errors.New("invalid embedding shape or encoding")
	}
	if err := validateEmbedding(embedding); err != nil {
		return EmbedResult{}, err
	}
	if !payload.Pose.Label.Valid() {
		return EmbedResult{}, ErrInvalidPose
	}
	if payload.QualityScore < 0 || payload.QualityScore > 1 {
		return EmbedResult{}, errors.New("invalid quality score")
	}

	return EmbedResult{
		QualityScore: payload.QualityScore,
		Pose:         payload.Pose,
		Template: StagedTemplate{
			Embedding:    embedding,
			EmbeddingDim: payload.EmbeddingDim,
			ModelName:    payload.ModelName,
			ModelVersion: payload.ModelVersion,
			QualityScore: payload.QualityScore,
		},
	}, nil
}

func validateEmbedding(embedding []byte) error {
	var normSquared float64
	for offset := 0; offset < len(embedding); offset += 4 {
		value := math.Float32frombits(binary.LittleEndian.Uint32(embedding[offset : offset+4]))
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return errors.New("embedding contains non-finite value")
		}
		normSquared += float64(value * value)
	}
	norm := math.Sqrt(normSquared)
	if math.Abs(norm-1) > 0.01 {
		return errors.New("embedding is not L2 normalized")
	}
	return nil
}
