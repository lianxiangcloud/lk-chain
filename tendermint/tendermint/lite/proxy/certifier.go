package proxy

import (
	"github.com/tendermint/tendermint/lite"
	"github.com/tendermint/tendermint/lite/files"
	certclient "github.com/tendermint/tendermint/lite/client"
)

func GetCertifier(chainID, rootDir, nodeAddr string) (*lite.Inquiring, error) {
	trust := lite.NewCacheProvider(
		lite.NewMemStoreProvider(),
		files.NewProvider(rootDir),
	)

	source := certclient.NewHTTPProvider(nodeAddr)

	// XXX: total insecure hack to avoid `init`
	fc, err := source.LatestCommit()
	if err != nil {
		return nil, err
	}
	cert := lite.NewInquiring(chainID, fc, trust, source)
	return cert, nil
}
