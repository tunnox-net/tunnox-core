package services

import (
	"context"

	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/services/client"
	"tunnox-core/internal/core/idgen"
)

func NewClientService(
	configRepo repos.IClientConfigRepository,
	stateRepo repos.IClientStateRepository,
	tokenRepo repos.IClientTokenRepository,
	clientRepo repos.IClientRepository,
	mappingRepo repos.IPortMappingRepository,
	idManager *idgen.IDManager,
	statsProvider StatsProvider,
	parentCtx context.Context,
) ClientService {
	return client.NewService(
		configRepo, stateRepo, tokenRepo,
		clientRepo, mappingRepo,
		idManager, statsProvider,
		parentCtx,
	)
}
