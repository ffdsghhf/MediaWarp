package emby

import (
	"MediaWarp/constants"
	"MediaWarp/utils"
	"encoding/json"
	"fmt" // 新增导入 fmt 用于错误格式化
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings" // 新增导入 strings 包
)

// 假设 EmbyItem 和 EmbyResponse 结构体已在项目的其他地方定义。
// 为便于理解，这里给出一个可能的最小定义示例：
/*
type EmbyItem struct {
    Id   string `json:"Id"`
    Name string `json:"Name"`
    Type string `json:"Type"`
    // ... 其他Emby媒体项包含的字段
}

type EmbyResponse struct {
    Items            []EmbyItem `json:"Items"`
    TotalRecordCount int        `json:"TotalRecordCount"`
    // StartIndex       int        `json:"StartIndex"` // 如果API响应中包含此字段
}
*/

type EmbyServer struct {
	endpoint string
	apiKey   string // 认证方式：APIKey；获取方式：Emby控制台 -> 高级 -> API密钥
}

// 获取媒体服务器类型
func (embyServer *EmbyServer) GetType() constants.MediaServerType {
	return constants.EMBY
}

// 获取EmbyServer连接地址
//
// 包含协议、服务器域名（IP）、端口号
// 示例：return "http://emby.example.com:8096"
func (embyServer *EmbyServer) GetEndpoint() string {
	return embyServer.endpoint
}

// 获取EmbyServer的API Key
func (embyServer *EmbyServer) GetAPIKey() string {
	return embyServer.apiKey
}

// ItemsService
// /Items
// 修改后的 ItemsServiceQueryItem 方法
func (embyServer *EmbyServer) ItemsServiceQueryItem(ids string, limit int, fields string) (*EmbyResponse, error) {
	// 初始化最终的响应对象，确保 Items 切片不为 nil
	finalItemResponse := &EmbyResponse{Items: make([]EmbyItem, 0)}

	// 如果传入的 ids 参数去除首尾空格后为空字符串，则直接返回空结果，
	// 避免向 Emby 发送空的 Ids 参数，这可能导致返回所有顶层项目或其他非预期行为。
	if strings.TrimSpace(ids) == "" {
		finalItemResponse.TotalRecordCount = 0
		return finalItemResponse, nil
	}

	// 按逗号分割 ids 字符串为 ID 列表
	idList := strings.Split(ids, ",")

	for _, singleId := range idList {
		trimmedId := strings.TrimSpace(singleId)
		// 跳过因连续逗号或首尾逗号产生的空 ID 字符串
		if trimmedId == "" {
			continue
		}

		//为每个单独的 ID 构建请求参数
		params := url.Values{}
		params.Add("Ids", trimmedId)
		// 由于我们是为每个 ID 单独查询，Limit 应设为 "1" 来获取该 ID 对应的单个媒体项。
		// 原始的 limit 参数将在所有结果聚合后用于限制最终返回的总数。
		params.Add("Limit", "1")
		params.Add("Fields", fields)
		params.Add("Recursive", "true")
		params.Add("api_key", embyServer.GetAPIKey())

		api := embyServer.GetEndpoint() + "/Items?" + params.Encode()
		resp, err := http.Get(api)
		if err != nil {
			// 如果发生网络错误（如无法连接服务器），则返回错误
			// 可以考虑更复杂的错误处理，例如重试或累积错误信息，但目前保持简单
			return nil, fmt.Errorf("请求媒体项 %s 时发生网络错误: %w", trimmedId, err)
		}

		// 确保在每次迭代的各种返回路径中都关闭响应体
		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close() // 关闭响应体
			continue          // 媒体项不存在，跳过当前 ID，继续处理下一个
		}

		if resp.StatusCode != http.StatusOK {
			// 如果收到非 200 OK 且非 404 NotFound 的状态码，表示可能发生其他错误
			// （如认证失败、服务器内部错误等）。
			// 读取响应体以获取更多错误信息，然后返回错误。
			bodyBytes, readErr := io.ReadAll(resp.Body)
			resp.Body.Close() // 关闭响应体

			errorMsg := fmt.Sprintf("查询媒体项 %s 时收到意外的状态码 %d", trimmedId, resp.StatusCode)
			if readErr == nil {
				errorMsg += fmt.Sprintf(". 响应内容: %s", string(bodyBytes))
			} else {
				errorMsg += fmt.Sprintf(". 读取响应体失败: %v", readErr)
			}
			return nil, fmt.Errorf(errorMsg)
		}

		// 读取响应体
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close() // 读取完毕后关闭响应体
		if readErr != nil {
			return nil, fmt.Errorf("读取媒体项 %s 的响应体失败: %w", trimmedId, readErr)
		}

		// 解析单个媒体项的响应
		// 假设即使是单个 ID 查询，Emby /Items 接口仍然返回 EmbyResponse 结构
		var itemResponsePart EmbyResponse
		if err := json.Unmarshal(body, &itemResponsePart); err != nil {
			return nil, fmt.Errorf("解析媒体项 %s 的JSON响应失败: %w. 响应体: %s", trimmedId, err, string(body))
		}

		// 如果成功获取到媒体项，则将其追加到最终结果的 Items 切片中
		if len(itemResponsePart.Items) > 0 {
			finalItemResponse.Items = append(finalItemResponse.Items, itemResponsePart.Items...)
		}
	}

	// 在所有单个 ID 请求处理完毕后，如果原始 limit 参数大于0，则对聚合后的结果进行数量限制
	if limit > 0 && len(finalItemResponse.Items) > limit {
		finalItemResponse.Items = finalItemResponse.Items[:limit]
	}

	// 更新最终响应中的 TotalRecordCount，以反映实际返回的媒体项数量
	finalItemResponse.TotalRecordCount = len(finalItemResponse.Items)

	return finalItemResponse, nil
}

// 获取index.html内容 API：/web/index.html
func (embyServer *EmbyServer) GetIndexHtml() ([]byte, error) {
	resp, err := http.Get(embyServer.GetEndpoint() + "/web/index.html")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() // 对于非循环的简单请求，defer 是安全的

	htmlContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return htmlContent, nil
}

// 获取EmbyServer实例
func New(addr string, apiKey string) *EmbyServer {
	emby := &EmbyServer{
		endpoint: utils.GetEndpoint(addr),
		apiKey:   apiKey,
	}
	return emby
}
