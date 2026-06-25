package handler

import (
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

func sessionMessageRequestPayload(req sendSessionMessageRequest) model.JSON {
	payload := model.JSON{}
	if len(req.AttachmentIDs) > 0 {
		ids := make([]any, 0, len(req.AttachmentIDs))
		for _, id := range req.AttachmentIDs {
			id = strings.TrimSpace(id)
			if id != "" {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			payload["attachment_ids"] = ids
		}
	}
	if len(req.CodeLineComments) > 0 {
		comments := make([]any, 0, len(req.CodeLineComments))
		for _, comment := range req.CodeLineComments {
			if len(comment) > 0 {
				comments = append(comments, comment)
			}
		}
		if len(comments) > 0 {
			payload["code_line_comments"] = comments
		}
	}
	return payload
}
