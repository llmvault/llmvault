package storage

import "fmt"

// addSheetPolicies registers the sheets feature's upload policies. Limits
// mirror sheets.MaxAttachmentFileBytes / MaxCSVUploadBytes.
func addSheetPolicies(policies map[AssetType]AssetPolicy) {
	sheetAttachmentTypes := map[string]string{
		"image/png":        "png",
		"image/jpeg":       "jpg",
		"image/webp":       "webp",
		"image/gif":        "gif",
		"application/pdf":  "pdf",
		"text/plain":       "txt",
		"text/csv":         "csv",
		"application/json": "json",
		"application/zip":  "zip",
	}
	csvTypes := map[string]string{
		"text/csv":        "csv",
		"application/csv": "csv",
		"text/plain":      "csv",
	}
	policies[AssetTypeSheetAttachment] = AssetPolicy{
		MaxBytes:     25 * 1024 * 1024,
		AllowedTypes: sheetAttachmentTypes,
		KeyPrefix: func(req SignRequest) (string, error) {
			if req.OrgID == nil {
				return "", fmt.Errorf("org_id is required for sheet_attachment")
			}
			return fmt.Sprintf("pub/o/%s/sheets/attachments/", *req.OrgID), nil
		},
	}
	policies[AssetTypeSheetImport] = AssetPolicy{
		MaxBytes:     50 * 1024 * 1024,
		AllowedTypes: csvTypes,
		KeyPrefix: func(req SignRequest) (string, error) {
			if req.OrgID == nil {
				return "", fmt.Errorf("org_id is required for sheet_import")
			}
			return fmt.Sprintf("pub/o/%s/sheetimports/", *req.OrgID), nil
		},
	}
}
