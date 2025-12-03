package api

// isSourceClientForMapping 判断是否是源端客户端（用于更新 bridge 的 sourceConn）
func (s *ManagementAPIServer) isSourceClientForMapping(mappingID string, clientID int64) bool {
	if s.cloudControl == nil || mappingID == "" || clientID == 0 {
		return false
	}

	mapping, err := s.cloudControl.GetPortMapping(mappingID)
	if err != nil {
		return false
	}

	listenClientID := mapping.ListenClientID
	if listenClientID == 0 {
		listenClientID = mapping.SourceClientID
	}

	return clientID == listenClientID
}
