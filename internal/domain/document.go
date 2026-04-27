package domain

import "time"

type DocType string

const (
    DocTypeContract  DocType = "contract"
    DocTypeFinancial DocType = "financial"
    DocTypeReport    DocType = "report"
    DocTypeOther     DocType = "other"
)

type Document struct {
    ID         string    `json:"id"`
    JVID       string    `json:"jv_id"`
    TenantID   string    `json:"tenant_id"`
    Name       string    `json:"name"`
    Type       DocType   `json:"type"`
    StorageKey string    `json:"storage_key"`
    UploadedBy string    `json:"uploaded_by"`
    UploadedAt time.Time `json:"uploaded_at"`
    Processed  bool      `json:"processed"`
}

func (d Document) IsProcessed() bool {
    return d.Processed
}
