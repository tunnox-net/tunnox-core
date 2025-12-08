package server

// setupContainer 设置依赖注入容器
func (s *Server) setupContainer() {
	// 注册核心服务到容器
	s.container.RegisterSingleton("session_manager", func() (interface{}, error) {
		return s.session, nil
	})

	if s.apiServer != nil {
		s.container.RegisterSingleton("http_server", func() (interface{}, error) {
			return s.apiServer, nil
		})
		// 注册 HTTPRouter 接口（依赖倒置原则：协议层依赖抽象接口）
		s.container.RegisterSingleton("http_router", func() (interface{}, error) {
			return s.apiServer, nil // ManagementAPIServer 实现了 HTTPRouter 接口
		})
	}

	s.container.RegisterSingleton("storage", func() (interface{}, error) {
		return s.storage, nil
	})

	if s.cloudControl != nil {
		s.container.RegisterSingleton("cloud_control", func() (interface{}, error) {
			return s.cloudControl, nil
		})
	}

	if s.messageBroker != nil {
		s.container.RegisterSingleton("message_broker", func() (interface{}, error) {
			return s.messageBroker, nil
		})
	}

	if s.bridgeManager != nil {
		s.container.RegisterSingleton("bridge_manager", func() (interface{}, error) {
			return s.bridgeManager, nil
		})
	}
}

