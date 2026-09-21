package face

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
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
