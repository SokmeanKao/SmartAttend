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

type ServiceError struct {
	Code       string
	Message    string
	StatusCode int
}

func (e *ServiceError) Error() string {
	return e.Code + ": " + e.Message
}

type DetectedPose struct {
	Label    Pose    `json:"label"`
	YawScore float32 `json:"yaw_score"`
}

type EmbedResult struct {
	QualityScore float32
	Pose         DetectedPose
	Template     StagedTemplate
}

type ReferenceTemplate struct {
	Pose Pose
	StagedTemplate
}

type VerifyResult struct {
	Matched      bool    `json:"matched"`
	BestScore    float32 `json:"best_score"`
	MatchedPose  Pose    `json:"matched_pose"`
	ModelName    string  `json:"model_name"`
	ModelVersion string  `json:"model_version"`
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
		return EmbedResult{}, faceServiceResponseError(response)
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

func (c *Client) Verify(
	ctx context.Context,
	image []byte,
	filename string,
	contentType string,
	templates []ReferenceTemplate,
) (VerifyResult, error) {
	references := struct {
		Templates []map[string]any `json:"templates"`
	}{Templates: make([]map[string]any, 0, len(templates))}
	for _, tmpl := range templates {
		references.Templates = append(references.Templates, map[string]any{
			"pose":               tmpl.Pose,
			"embedding":          base64.StdEncoding.EncodeToString(tmpl.Embedding),
			"embedding_encoding": "float32-le-base64",
			"embedding_dim":      tmpl.EmbeddingDim,
			"model_name":         tmpl.ModelName,
			"model_version":      tmpl.ModelVersion,
		})
	}
	referencesJSON, err := json.Marshal(references)
	if err != nil {
		return VerifyResult{}, err
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", filename)
	if err != nil {
		return VerifyResult{}, err
	}
	if _, err := part.Write(image); err != nil {
		return VerifyResult{}, err
	}
	if err := writer.WriteField("references", string(referencesJSON)); err != nil {
		return VerifyResult{}, err
	}
	if err := writer.Close(); err != nil {
		return VerifyResult{}, err
	}

	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"/internal/v1/faces/verify", &body,
	)
	if err != nil {
		return VerifyResult{}, err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if contentType != "" {
		request.Header.Set("X-Image-Content-Type", contentType)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return VerifyResult{}, ErrFaceServiceTimeout
		}
		return VerifyResult{}, fmt.Errorf("%w: %v", ErrFaceServiceUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return VerifyResult{}, faceServiceResponseError(response)
	}
	var result VerifyResult
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return VerifyResult{}, fmt.Errorf("%w: invalid response", ErrFaceServiceUnavailable)
	}
	if result.Matched && !result.MatchedPose.Valid() {
		return VerifyResult{}, fmt.Errorf("%w: invalid matched pose", ErrFaceServiceUnavailable)
	}
	if math.IsNaN(float64(result.BestScore)) || math.IsInf(float64(result.BestScore), 0) {
		return VerifyResult{}, fmt.Errorf("%w: invalid score", ErrFaceServiceUnavailable)
	}
	return result, nil
}

func faceServiceResponseError(response *http.Response) error {
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err == nil &&
		payload.Error.Code != "" &&
		payload.Error.Message != "" {
		return &ServiceError{
			Code:       payload.Error.Code,
			Message:    payload.Error.Message,
			StatusCode: response.StatusCode,
		}
	}
	if response.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("%w: status %d", ErrFaceServiceUnavailable, response.StatusCode)
	}
	return fmt.Errorf("face service returned status %d without an error envelope", response.StatusCode)
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
