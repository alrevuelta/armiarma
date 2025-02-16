package endpoint

import (
	"context"

	"github.com/migalabs/armiarma/src/onchaindata/eth2/endpoint/types"
	"github.com/pkg/errors"
)

func (c *InfuraClient) ReqGenesis(ctx context.Context) (gen types.Genesis, err error) {
	if !c.IsInitialized() {
		return gen, errors.New("infura client is not initialized")
	}
	err = c.NewHttpsRequest(ctx, GENESIS_ENPOINT, &gen)
	return gen, err
}
