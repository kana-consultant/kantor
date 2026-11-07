package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

type mockS3Client struct {
	objects map[string][]byte
}

func newMockS3Client() *mockS3Client {
	return &mockS3Client{
		objects: make(map[string][]byte),
	}
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	data, err := io.ReadAll(params.Body)
	if err != nil {
		return nil, err
	}
	m.objects[*params.Key] = data
	return &s3.PutObjectOutput{}, nil
}

func (m *mockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	data, ok := m.objects[*params.Key]
	if !ok {
		return nil, &s3types.NoSuchKey{}
	}
	now := time.Now()
	return &s3.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewReader(data)),
		ContentLength: aws.Int64(int64(len(data))),
		ContentType:   aws.String("application/octet-stream"),
		LastModified:  &now,
	}, nil
}

func (m *mockS3Client) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	delete(m.objects, *params.Key)
	return &s3.DeleteObjectOutput{}, nil
}

func (m *mockS3Client) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	data, ok := m.objects[*params.Key]
	if !ok {
		return nil, &s3types.NotFound{}
	}
	return &s3.HeadObjectOutput{
		ContentLength: aws.Int64(int64(len(data))),
	}, nil
}

func TestS3Storage_CRUD(t *testing.T) {
	mockClient := newMockS3Client()
	store := NewS3StorageWithClient(mockClient, "my-bucket", "test-prefix")
	ctx := context.Background()

	key := "tenant_abc/reimbursements/receipt.pdf"
	data := []byte("%PDF-dummy-content")

	// 1. Put
	if err := store.Put(ctx, key, bytes.NewReader(data), int64(len(data)), "application/pdf"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// 2. Exists
	exists, err := store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists error: %v", err)
	}
	if !exists {
		t.Fatalf("expected file to exist")
	}

	// 3. Get
	rc, meta, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("expected %q, got %q", data, got)
	}
	if meta.ContentLength != int64(len(data)) {
		t.Fatalf("expected length %d, got %d", len(data), meta.ContentLength)
	}

	// 4. Delete
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	// 5. Exists after delete
	existsAfter, err := store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists after delete: %v", err)
	}
	if existsAfter {
		t.Fatalf("expected object not to exist after delete")
	}

	// 6. Get after delete
	_, _, err = store.Get(ctx, key)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestS3Storage_InvalidPath(t *testing.T) {
	mockClient := newMockS3Client()
	store := NewS3StorageWithClient(mockClient, "my-bucket", "")
	ctx := context.Background()

	badKeys := []string{"", "  ", "/root.png", "../escape.png"}
	for _, k := range badKeys {
		if err := store.Put(ctx, k, bytes.NewReader([]byte("test")), 4, "text/plain"); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("expected ErrInvalidPath for key %q, got: %v", k, err)
		}
	}
}

func TestIsS3NotFound(t *testing.T) {
	if !isS3NotFound(&s3types.NoSuchKey{}) {
		t.Errorf("expected NoSuchKey to be not found")
	}
	if !isS3NotFound(&s3types.NotFound{}) {
		t.Errorf("expected NotFound to be not found")
	}
	apiErr := &smithy.GenericAPIError{
		Code: "NoSuchKey",
	}
	if !isS3NotFound(apiErr) {
		t.Errorf("expected GenericAPIError NoSuchKey to be not found")
	}
	if isS3NotFound(errors.New("other error")) {
		t.Errorf("expected regular error not to be not found")
	}
}
