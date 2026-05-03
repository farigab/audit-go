package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"
)

var (
	ErrBlobStorageNotConfigured = errors.New("blob storage is not configured")
	ErrBlobNotFound             = errors.New("blob not found")
)

// AzureBlobConfig contains Azure Blob Storage upload settings.
type AzureBlobConfig struct {
	AccountName string
	Container   string
	Endpoint    string
}

// UploadURL describes a client-side direct upload target.
type UploadURL struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expires_at"`
	Container string            `json:"-"`
}

// BlobProperties contains verified blob metadata used for chain of custody.
type BlobProperties struct {
	Container   string
	StorageKey  string
	ContentType string
	SizeBytes   int64
	ETag        string
	VersionID   string
}

// AzureBlobStore creates user delegation upload URLs and verifies uploaded blobs.
type AzureBlobStore struct {
	accountName string
	container   string
	endpoint    string
	client      *service.Client
}

// NewAzureBlobStore creates a Blob Storage client using DefaultAzureCredential.
func NewAzureBlobStore(cfg AzureBlobConfig) (*AzureBlobStore, error) {
	if cfg.AccountName == "" || cfg.Container == "" {
		return nil, ErrBlobStorageNotConfigured
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://%s.blob.core.windows.net/", cfg.AccountName)
	}

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("creating default azure credential: %w", err)
	}

	client, err := service.NewClient(endpoint, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating blob service client: %w", err)
	}

	return &AzureBlobStore{
		accountName: cfg.AccountName,
		container:   cfg.Container,
		endpoint:    strings.TrimRight(endpoint, "/"),
		client:      client,
	}, nil
}

// ContainerName returns the container used by the store.
func (s *AzureBlobStore) ContainerName() string {
	if s == nil {
		return ""
	}
	return s.container
}

// CreateUploadURL creates a short-lived user delegation SAS for one block blob.
func (s *AzureBlobStore) CreateUploadURL(
	ctx context.Context,
	storageKey string,
	contentType string,
	expiresAt time.Time,
) (UploadURL, error) {
	if s == nil {
		return UploadURL{}, ErrBlobStorageNotConfigured
	}

	now := time.Now().UTC()
	start := now.Add(-1 * time.Minute)
	expiry := expiresAt.UTC()
	if !expiry.After(now) {
		return UploadURL{}, fmt.Errorf("upload expiry must be in the future")
	}

	info := service.KeyInfo{
		Start:  to.Ptr(start.Format(sas.TimeFormat)),
		Expiry: to.Ptr(expiry.Format(sas.TimeFormat)),
	}

	udc, err := s.client.GetUserDelegationCredential(ctx, info, nil)
	if err != nil {
		return UploadURL{}, fmt.Errorf("getting user delegation credential: %w", err)
	}

	query, err := sas.BlobSignatureValues{
		Protocol:      sas.ProtocolHTTPS,
		StartTime:     start,
		ExpiryTime:    expiry,
		Permissions:   to.Ptr(sas.BlobPermissions{Create: true, Write: true}).String(),
		ContainerName: s.container,
		BlobName:      storageKey,
	}.SignWithUserDelegation(udc)
	if err != nil {
		return UploadURL{}, fmt.Errorf("signing upload sas: %w", err)
	}

	headers := map[string]string{
		"x-ms-blob-type": "BlockBlob",
	}
	if contentType != "" {
		headers["Content-Type"] = contentType
		headers["x-ms-blob-content-type"] = contentType
	}

	return UploadURL{
		Method:    "PUT",
		URL:       s.blobURL(storageKey) + "?" + query.Encode(),
		Headers:   headers,
		ExpiresAt: expiry,
		Container: s.container,
	}, nil
}

// GetProperties verifies the blob exists and returns metadata from Blob Storage.
func (s *AzureBlobStore) GetProperties(ctx context.Context, storageKey string) (BlobProperties, error) {
	if s == nil {
		return BlobProperties{}, ErrBlobStorageNotConfigured
	}

	props, err := s.client.NewContainerClient(s.container).NewBlobClient(storageKey).GetProperties(ctx, nil)
	if err != nil {
		if bloberror.HasCode(err, bloberror.BlobNotFound, bloberror.ContainerNotFound) {
			return BlobProperties{}, ErrBlobNotFound
		}
		return BlobProperties{}, fmt.Errorf("getting blob properties: %w", err)
	}

	var contentType string
	if props.ContentType != nil {
		contentType = *props.ContentType
	}

	var size int64
	if props.ContentLength != nil {
		size = *props.ContentLength
	}

	var etag string
	if props.ETag != nil {
		etag = string(*props.ETag)
	}

	var versionID string
	if props.VersionID != nil {
		versionID = *props.VersionID
	}

	return BlobProperties{
		Container:   s.container,
		StorageKey:  storageKey,
		ContentType: contentType,
		SizeBytes:   size,
		ETag:        etag,
		VersionID:   versionID,
	}, nil
}

// Download reads a blob and returns its bytes with storage metadata.
func (s *AzureBlobStore) Download(ctx context.Context, storageKey string) (DownloadedBlob, error) {
	if s == nil {
		return DownloadedBlob{}, ErrBlobStorageNotConfigured
	}

	resp, err := s.client.NewContainerClient(s.container).NewBlobClient(storageKey).DownloadStream(ctx, nil)
	if err != nil {
		if bloberror.HasCode(err, bloberror.BlobNotFound, bloberror.ContainerNotFound) {
			return DownloadedBlob{}, ErrBlobNotFound
		}
		return DownloadedBlob{}, fmt.Errorf("downloading blob: %w", err)
	}

	body := resp.NewRetryReader(ctx, nil)
	defer func() { _ = body.Close() }()

	content, err := io.ReadAll(body)
	if err != nil {
		return DownloadedBlob{}, fmt.Errorf("reading blob content: %w", err)
	}

	var contentType string
	if resp.ContentType != nil {
		contentType = *resp.ContentType
	}

	var size int64
	if resp.ContentLength != nil {
		size = *resp.ContentLength
	}

	var etag string
	if resp.ETag != nil {
		etag = string(*resp.ETag)
	}

	var versionID string
	if resp.VersionID != nil {
		versionID = *resp.VersionID
	}

	return DownloadedBlob{
		Content: content,
		Properties: BlobProperties{
			Container:   s.container,
			StorageKey:  storageKey,
			ContentType: contentType,
			SizeBytes:   size,
			ETag:        etag,
			VersionID:   versionID,
		},
	}, nil
}

func (s *AzureBlobStore) blobURL(storageKey string) string {
	return s.endpoint + "/" + pathEscape(s.container) + "/" + escapeBlobPath(storageKey)
}

func escapeBlobPath(value string) string {
	segments := strings.Split(value, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}

func pathEscape(value string) string {
	return url.PathEscape(strings.Trim(value, "/"))
}
