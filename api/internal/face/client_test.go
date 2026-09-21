package face

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientEmbedSendsImageAndReturnsValidatedTemplate(t *testing.T) {
	embedding := make([]byte, 128*4)
	binary.LittleEndian.PutUint32(embedding[:4], math.Float32bits(1))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/faces/embed" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}
		if got := r.FormValue("expected_pose"); got != "LEFT" {
			t.Fatalf("expected_pose = %q, want LEFT", got)
		}
		file, _, err := r.FormFile("image")
		if err != nil {
			t.Fatalf("FormFile() error = %v", err)
		}
		defer file.Close()
		if got, _ := io.ReadAll(file); string(got) != "image-bytes" {
			t.Fatalf("image = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"quality_score":      0.91,
			"pose":               map[string]any{"label": "LEFT", "yaw_score": -0.4},
			"embedding":          base64.StdEncoding.EncodeToString(embedding),
			"embedding_encoding": "float32-le-base64",
			"embedding_dim":      128,
			"model_name":         "sface",
			"model_version":      "2021dec",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, time.Second)
	result, err := client.Embed(context.Background(), []byte("image-bytes"), "face.jpg", "image/jpeg", PoseLeft)
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(result.Template.Embedding) != 512 || result.Template.EmbeddingDim != 128 {
		t.Fatalf("template = %#v", result.Template)
	}
	if result.Pose.Label != PoseLeft || result.QualityScore != 0.91 {
		t.Fatalf("result = %#v", result)
	}
}

func TestClientEmbedRejectsMalformedEmbedding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"quality_score":      0.91,
			"pose":               map[string]any{"label": "FRONT", "yaw_score": 0},
			"embedding":          base64.StdEncoding.EncodeToString([]byte("short")),
			"embedding_encoding": "float32-le-base64",
			"embedding_dim":      128,
			"model_name":         "sface",
			"model_version":      "2021dec",
		})
	}))
	defer server.Close()

	_, err := NewClient(server.URL, time.Second).Embed(
		context.Background(), []byte("image"), "face.jpg", "image/jpeg", PoseFront,
	)
	if err == nil {
		t.Fatal("Embed() error = nil, want malformed embedding error")
	}
}

func TestClientEmbedPropagatesFaceServiceErrorEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"code":    "FACE_NOT_FOUND",
				"message": "no qualifying face was detected",
			},
		})
	}))
	defer server.Close()

	_, err := NewClient(server.URL, time.Second).Embed(
		context.Background(), []byte("image"), "face.jpg", "image/jpeg", PoseFront,
	)
	var serviceErr *ServiceError
	if !errors.As(err, &serviceErr) {
		t.Fatalf("Embed() error = %v, want *ServiceError", err)
	}
	if serviceErr.Code != "FACE_NOT_FOUND" ||
		serviceErr.Message != "no qualifying face was detected" ||
		serviceErr.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("service error = %#v", serviceErr)
	}
	if errors.Is(err, ErrFaceServiceUnavailable) {
		t.Fatal("error envelope was collapsed to ErrFaceServiceUnavailable")
	}
}

func TestClientEmbedMapsBareServerErrorToUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream failed", http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := NewClient(server.URL, time.Second).Embed(
		context.Background(), []byte("image"), "face.jpg", "image/jpeg", PoseFront,
	)
	if !errors.Is(err, ErrFaceServiceUnavailable) {
		t.Fatalf("Embed() error = %v, want ErrFaceServiceUnavailable", err)
	}
}

func TestClientVerifySendsReferencesAndReturnsDecision(t *testing.T) {
	embedding := make([]byte, 128*4)
	binary.LittleEndian.PutUint32(embedding[:4], math.Float32bits(1))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/faces/verify" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}
		var references struct {
			Templates []map[string]any `json:"templates"`
		}
		if err := json.Unmarshal([]byte(r.FormValue("references")), &references); err != nil {
			t.Fatalf("references JSON error = %v", err)
		}
		if len(references.Templates) != 1 ||
			references.Templates[0]["embedding"] == "" {
			t.Fatalf("references = %#v", references)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"matched":       true,
			"best_score":    0.91,
			"matched_pose":  "FRONT",
			"model_name":    "sface",
			"model_version": "2021dec",
		})
	}))
	defer server.Close()

	result, err := NewClient(server.URL, time.Second).Verify(
		context.Background(),
		[]byte("image-bytes"),
		"face.jpg",
		"image/jpeg",
		[]ReferenceTemplate{{
			Pose: PoseFront,
			StagedTemplate: StagedTemplate{
				Embedding: embedding, EmbeddingDim: 128,
				ModelName: "sface", ModelVersion: "2021dec",
			},
		}},
	)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !result.Matched || result.BestScore != 0.91 || result.MatchedPose != PoseFront {
		t.Fatalf("result = %#v", result)
	}
}
