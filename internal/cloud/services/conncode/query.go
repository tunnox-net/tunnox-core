package conncode

import (
	"errors"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/utils/random"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 连接码查询方法
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ListConnectionCodesByTargetClient 列出TargetClient的连接码
//
// 返回指定TargetClient生成的所有连接码（已过期的未激活连接码会被过滤）
func (s *Service) ListConnectionCodesByTargetClient(targetClientID int64) ([]*models.TunnelConnectionCode, error) {
	codes, err := s.connCodeRepo.ListByTargetClient(targetClientID)
	if err != nil {
		return nil, err
	}

	// 过滤掉已过期且未激活的连接码
	filtered := make([]*models.TunnelConnectionCode, 0, len(codes))
	for _, code := range codes {
		// 已过期且未激活的连接码不返回（会被清理）
		if code.IsExpired() && !code.IsActivated {
			// 异步清理过期的连接码
			go func(c *models.TunnelConnectionCode) {
				select {
				case <-s.Ctx().Done():
					return
				default:
					if err := s.connCodeRepo.Delete(c.ID); err != nil {
						corelog.Debugf("ConnectionCodeService: failed to cleanup expired code %s: %v", c.Code, err)
					}
				}
			}(code)
			continue
		}
		filtered = append(filtered, code)
	}

	return filtered, nil
}

// GetConnectionCode 获取连接码详情
func (s *Service) GetConnectionCode(code string) (*models.TunnelConnectionCode, error) {
	connCode, err := s.connCodeRepo.GetByCode(code)
	if err != nil {
		if errors.Is(err, repos.ErrNotFound) {
			return nil, coreerrors.New(coreerrors.CodeNotFound, "connection code not found or expired")
		}
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get connection code")
	}
	return connCode, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 映射查询方法
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ListOutboundMappings 列出出站映射（ListenClient创建的映射）
//
// 返回指定ListenClient创建的所有映射（我在访问谁）
func (s *Service) ListOutboundMappings(listenClientID int64) ([]*models.PortMapping, error) {
	clientKey := random.Int64ToString(listenClientID)
	corelog.Debugf("ConnectionCodeService.ListOutboundMappings: querying mappings for client %d (key=%s)", listenClientID, clientKey)

	allMappings, err := s.portMappingRepo.GetClientPortMappings(clientKey)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get client port mappings")
	}

	corelog.Debugf("ConnectionCodeService.ListOutboundMappings: found %d mappings from index for client %d", len(allMappings), listenClientID)

	// 过滤出 ListenClientID 匹配的映射
	result := make([]*models.PortMapping, 0)
	for _, m := range allMappings {
		if m.ListenClientID == listenClientID {
			corelog.Debugf("ConnectionCodeService.ListOutboundMappings: adding mapping %s (ListenClientID=%d)", m.ID, m.ListenClientID)
			result = append(result, m)
		} else {
			corelog.Debugf("ConnectionCodeService.ListOutboundMappings: skipping mapping %s (ListenClientID=%d != %d)", m.ID, m.ListenClientID, listenClientID)
		}
	}

	corelog.Debugf("ConnectionCodeService.ListOutboundMappings: returning %d outbound mappings for client %d", len(result), listenClientID)
	return result, nil
}

// ListInboundMappings 列出入站映射（通过TargetClient的连接码创建的映射）
//
// 返回访问指定TargetClient的所有映射（谁在访问我）
func (s *Service) ListInboundMappings(targetClientID int64) ([]*models.PortMapping, error) {
	clientKey := random.Int64ToString(targetClientID)
	corelog.Debugf("ConnectionCodeService.ListInboundMappings: querying mappings for client %d (key=%s)", targetClientID, clientKey)

	allMappings, err := s.portMappingRepo.GetClientPortMappings(clientKey)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get client port mappings")
	}

	corelog.Debugf("ConnectionCodeService.ListInboundMappings: found %d mappings from index for client %d", len(allMappings), targetClientID)

	// 过滤出 TargetClientID 匹配的映射
	result := make([]*models.PortMapping, 0)
	for _, m := range allMappings {
		if m.TargetClientID == targetClientID {
			corelog.Debugf("ConnectionCodeService.ListInboundMappings: adding mapping %s (TargetClientID=%d)", m.ID, m.TargetClientID)
			result = append(result, m)
		} else {
			corelog.Debugf("ConnectionCodeService.ListInboundMappings: skipping mapping %s (TargetClientID=%d != %d)", m.ID, m.TargetClientID, targetClientID)
		}
	}

	corelog.Debugf("ConnectionCodeService.ListInboundMappings: returning %d inbound mappings for client %d", len(result), targetClientID)
	return result, nil
}

// GetMapping 获取映射详情
func (s *Service) GetMapping(mappingID string) (*models.PortMapping, error) {
	return s.portMappingService.GetPortMapping(mappingID)
}
