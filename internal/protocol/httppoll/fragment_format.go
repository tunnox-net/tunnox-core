package httppoll

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// FragmentResponse 统一的分片格式（用于 Request 和 Response）
// 判断是否为分片：total_fragments > 1
type FragmentResponse struct {
	FragmentGroupID string `json:"fragment_group_id"` // 分片组ID（UUID）
	OriginalSize    int    `json:"original_size"`     // 原始字节流大小
	FragmentSize    int    `json:"fragment_size"`     // 当前分片字节流大小
	FragmentIndex   int    `json:"fragment_index"`    // 当前是第几片（从0开始）
	TotalFragments  int    `json:"total_fragments"`   // 总共有多少片
	SequenceNumber  int64  `json:"sequence_number"`   // 序列号（用于保证数据包顺序，同一WriteExact调用的所有分片共享相同序列号）
	Data            string `json:"data"`              // Base64编码的分片数据
	Timestamp       int64  `json:"timestamp"`         // 时间戳

	// 仅 Response 使用
	Success bool `json:"success,omitempty"` // 响应是否成功
	Timeout bool `json:"timeout,omitempty"` // 是否超时
}

// CreateFragmentResponse 创建分片响应
func CreateFragmentResponse(data []byte, fragmentIndex int, fragmentSize int, totalFragments int, originalSize int, groupID string, sequenceNumber int64) *FragmentResponse {
	// 获取分片数据
	fragmentData := GetFragmentData(data, fragmentIndex, fragmentSize, totalFragments)
	if fragmentData == nil {
		return nil
	}

	// Base64编码
	base64Data := base64.StdEncoding.EncodeToString(fragmentData)

	return &FragmentResponse{
		FragmentGroupID: groupID,
		OriginalSize:    originalSize,
		FragmentSize:    len(fragmentData),
		FragmentIndex:   fragmentIndex,
		TotalFragments:  totalFragments,
		SequenceNumber:  sequenceNumber,
		Data:            base64Data,
		Success:         true,
		Timestamp:       time.Now().Unix(),
	}
}

// CreateCompleteResponse 创建完整数据响应（不分片）
func CreateCompleteResponse(data []byte, sequenceNumber int64) *FragmentResponse {
	groupID := uuid.New().String()
	base64Data := base64.StdEncoding.EncodeToString(data)

	return &FragmentResponse{
		FragmentGroupID: groupID,
		OriginalSize:    len(data),
		FragmentSize:    len(data),
		FragmentIndex:   0,
		TotalFragments:  1,
		SequenceNumber:  sequenceNumber,
		Data:            base64Data,
		Success:         true,
		Timestamp:       time.Now().Unix(),
	}
}

// CreateTimeoutResponse 创建超时响应
func CreateTimeoutResponse() *FragmentResponse {
	return &FragmentResponse{
		Success:   true,
		Timeout:   true,
		Timestamp: time.Now().Unix(),
	}
}

// MarshalFragmentResponse 序列化分片响应
func MarshalFragmentResponse(resp *FragmentResponse) ([]byte, error) {
	return json.Marshal(resp)
}

// UnmarshalFragmentResponse 反序列化分片响应
func UnmarshalFragmentResponse(data []byte) (*FragmentResponse, error) {
	var resp FragmentResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal fragment response: %w", err)
	}
	return &resp, nil
}

// SplitDataIntoFragments 将数据分片
func SplitDataIntoFragments(data []byte, sequenceNumber int64) ([]*FragmentResponse, error) {
	dataSize := len(data)
	fragmentSize, totalFragments := CalculateFragments(dataSize)

	if totalFragments == 1 {
		// 不分片，直接返回完整数据
		return []*FragmentResponse{CreateCompleteResponse(data, sequenceNumber)}, nil
	}

	// 生成分片组ID
	groupID := uuid.New().String()

	// 创建分片响应列表
	fragments := make([]*FragmentResponse, 0, totalFragments)
	for i := 0; i < totalFragments; i++ {
		fragmentResp := CreateFragmentResponse(data, i, fragmentSize, totalFragments, dataSize, groupID, sequenceNumber)
		if fragmentResp == nil {
			return nil, fmt.Errorf("failed to create fragment %d", i)
		}
		fragments = append(fragments, fragmentResp)
	}

	return fragments, nil
}
