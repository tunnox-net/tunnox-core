package repos

import (
	constants2 "tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/constants"
)

// 编译时接口断言，确保 NodeRepository 实现了 INodeRepository 接口
var _ INodeRepository = (*NodeRepository)(nil)

// NodeRepository 节点数据访问
type NodeRepository struct {
	*GenericRepositoryImpl[*models.Node]
}

// NewNodeRepository 创建节点数据访问层
func NewNodeRepository(repo *Repository) *NodeRepository {
	genericRepo := NewGenericRepository[*models.Node](repo, func(node *models.Node) (string, error) {
		return node.ID, nil
	})
	return &NodeRepository{GenericRepositoryImpl: genericRepo}
}

// SaveNode 保存节点（创建或更新）
func (r *NodeRepository) SaveNode(node *models.Node) error {
	return r.Save(node, constants.KeyPrefixNode, constants2.DefaultNodeDataTTL)
}

// CreateNode 创建新节点（仅创建，不允许覆盖）
func (r *NodeRepository) CreateNode(node *models.Node) error {
	return r.Create(node, constants.KeyPrefixNode, constants2.DefaultNodeDataTTL)
}

// UpdateNode 更新节点（仅更新，不允许创建）
func (r *NodeRepository) UpdateNode(node *models.Node) error {
	return r.Update(node, constants.KeyPrefixNode, constants2.DefaultNodeDataTTL)
}

// GetNode 获取节点
func (r *NodeRepository) GetNode(nodeID string) (*models.Node, error) {
	return r.Get(nodeID, constants.KeyPrefixNode)
}

// DeleteNode 删除节点
func (r *NodeRepository) DeleteNode(nodeID string) error {
	return r.Delete(nodeID, constants.KeyPrefixNode)
}

// ListNodes 列出所有节点
func (r *NodeRepository) ListNodes() ([]*models.Node, error) {
	return r.List(constants.KeyPrefixNodeList)
}

// AddNodeToList 添加节点到列表
func (r *NodeRepository) AddNodeToList(node *models.Node) error {
	return r.AddToList(node, constants.KeyPrefixNodeList)
}
