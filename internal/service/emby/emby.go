package emby

import (
	"MediaWarp/constants" // Assuming this path is correct
	"MediaWarp/utils"    // Assuming this path is correct
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Note: EmbyResponse and BaseItemDto (and other related types) are now expected
// to be defined in internal/service/emby/schema.go

// EmbyServer struct definition
type EmbyServer struct {
	endpoint string
	apiKey   string
}

// GetType gets the media server type
func (embyServer *EmbyServer) GetType() constants.MediaServerType {
	return constants.EMBY
}

// GetEndpoint gets the EmbyServer connection address
func (embyServer *EmbyServer) GetEndpoint() string {
	return embyServer.endpoint
}

// GetAPIKey gets the EmbyServer API Key
func (embyServer *EmbyServer) GetAPIKey() string {
	return embyServer.apiKey
}

// ItemsServiceQueryItem queries media items
// This method has been modified to handle potentially invalid IDs in the ids list
// by querying each ID individually.
func (embyServer *EmbyServer) ItemsServiceQueryItem(ids string, limit int, fields string) (*EmbyResponse, error) {
	// Initialize TotalRecordCount as a pointer to int64(0)
	initialCount := int64(0)
	finalItemResponse := &EmbyResponse{
		Items:            make([]BaseItemDto, 0), // Use BaseItemDto from schema.go
		TotalRecordCount: &initialCount,
	}

	trimmedIds := strings.TrimSpace(ids)
	if trimmedIds == "" {
		// If the incoming ids parameter is empty after trimming, return the initialized empty response
		return finalItemResponse, nil
	}

	idList := strings.Split(trimmedIds, ",")
	collectedItems := make([]BaseItemDto, 0, len(idList)) // Use BaseItemDto

	for _, singleId := range idList {
		trimmedSingleId := strings.TrimSpace(singleId)
		if trimmedSingleId == "" {
			continue
		}

		params := url.Values{}
		params.Add("Ids", trimmedSingleId)
		params.Add("Limit", "1")
		params.Add("Fields", fields)
		params.Add("Recursive", "true") // Key parameter from the issue
		params.Add("api_key", embyServer.GetAPIKey())

		api := embyServer.GetEndpoint() + "/Items?" + params.Encode()
		resp, err := http.Get(api)
		if err != nil {
			return nil, fmt.Errorf("请求媒体项 %s 时发生网络错误: %w", trimmedSingleId, err)
		}

		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			continue
		}

		if resp.StatusCode != http.StatusOK {
			bodyBytes, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			errorMsg := fmt.Sprintf("查询媒体项 %s 时收到意外的状态码 %d", trimmedSingleId, resp.StatusCode)
			if readErr == nil {
				errorMsg += fmt.Sprintf(". 响应内容: %s", string(bodyBytes))
			} else {
				errorMsg += fmt.Sprintf(". 读取响应体失败: %v", readErr)
			}
			return nil, fmt.Errorf(errorMsg)
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("读取媒体项 %s 的响应体失败: %w", trimmedSingleId, readErr)
		}

		var itemResponsePart EmbyResponse // This EmbyResponse is from schema.go
		if err := json.Unmarshal(body, &itemResponsePart); err != nil {
			return nil, fmt.Errorf("解析媒体项 %s 的JSON响应失败: %w. 响应体: %s", trimmedSingleId, err, string(body))
		}

		// itemResponsePart.Items will be []BaseItemDto
		if len(itemResponsePart.Items) > 0 {
			collectedItems = append(collectedItems, itemResponsePart.Items...)
		}
	}

	if limit > 0 && len(collectedItems) > limit {
		finalItemResponse.Items = collectedItems[:limit]
	} else {
		finalItemResponse.Items = collectedItems
	}

	// Update TotalRecordCount. Since it's *int64, assign the address of the count.
	finalCount := int64(len(finalItemResponse.Items))
	finalItemResponse.TotalRecordCount = &finalCount

	return finalItemResponse, nil
}

// GetIndexHtml gets index.html content API: /web/index.html
func (embyServer *EmbyServer) GetIndexHtml() ([]byte, error) {
	resp, err := http.Get(embyServer.GetEndpoint() + "/web/index.html")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	htmlContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return htmlContent, nil
}

// New gets an EmbyServer instance
func New(addr string, apiKey string) *EmbyServer {
	emby := &EmbyServer{
		endpoint: utils.GetEndpoint(addr), // Assuming utils.GetEndpoint exists and works correctly
		apiKey:   apiKey,
	}
	return emby
}
