package node

import (
	"errors"
	"fmt"
	"net"

	//"os"
	"encoding/json"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"strings"
	"time"

	abci "github.com/tendermint/abci/types"
	crypto "github.com/tendermint/go-crypto"
	wire "github.com/tendermint/go-wire"
	bc "github.com/tendermint/tendermint/blockchain"
	cfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/consensus"
	"github.com/tendermint/tendermint/debug"
	"github.com/tendermint/tendermint/evidence"
	mempl "github.com/tendermint/tendermint/mempool"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/p2p/trust"
	"github.com/tendermint/tendermint/proxy"
	rpccore "github.com/tendermint/tendermint/rpc/core"
	grpccore "github.com/tendermint/tendermint/rpc/grpc"
	rpc "github.com/tendermint/tendermint/rpc/lib"
	rpcserver "github.com/tendermint/tendermint/rpc/lib/server"
	sm "github.com/tendermint/tendermint/state"
	"github.com/tendermint/tendermint/state/txindex"
	"github.com/tendermint/tendermint/state/txindex/kv"
	"github.com/tendermint/tendermint/state/txindex/null"
	"github.com/tendermint/tendermint/types"
	"github.com/tendermint/tendermint/version"
	cmn "github.com/tendermint/tmlibs/common"
	dbm "github.com/tendermint/tmlibs/db"
	"github.com/tendermint/tmlibs/log"
)

//------------------------------------------------------------------------------

// DBContext specifies config information for loading a new DB.
type DBContext struct {
	ID     string
	Config *cfg.Config
}

// DBProvider takes a DBContext and returns an instantiated DB.
type DBProvider func(*DBContext) (dbm.DB, error)

// DefaultDBProvider returns a database using the DBBackend and DBDir
// specified in the ctx.Config.
func DefaultDBProvider(ctx *DBContext) (dbm.DB, error) {
	return dbm.NewDB(ctx.ID, ctx.Config.DBBackend, ctx.Config.DBDir()), nil
}

func CustomizedDBProvider(id, dbBackend, dbDir string) (dbm.DB, error) {
	return dbm.NewDB(id, dbBackend, dbDir), nil
}

// GenesisDocProvider returns a GenesisDoc.
// It allows the GenesisDoc to be pulled from sources other than the
// filesystem, for instance from a distributed key-value store cluster.
type GenesisDocProvider func() (*types.GenesisDoc, error)

// DefaultGenesisDocProviderFunc returns a GenesisDocProvider that loads
// the GenesisDoc from the config.GenesisFile() on the filesystem.
func DefaultGenesisDocProviderFunc(config *cfg.Config) GenesisDocProvider {
	return func() (*types.GenesisDoc, error) {
		return types.GenesisDocFromFile(config.GenesisFile())
	}
}

// NodeProvider takes a config and a logger and returns a ready to go Node.
type NodeProvider func(*cfg.Config, log.Logger) (*Node, error)

// DefaultNewNode returns a Tendermint node with default settings for the
// PrivValidator, ClientCreator, GenesisDoc, and DBProvider.
// It implements NodeProvider.
func DefaultNewNode(config *cfg.Config, logger log.Logger) (*Node, error) {
	return NewNode(config,
		types.LoadOrGenPrivValidatorFS(config.PrivValidatorFile()),
		proxy.DefaultClientCreator(config.ProxyApp, config.ABCI, config.DBDir()),
		DefaultGenesisDocProviderFunc(config),
		DefaultDBProvider,
		logger)
}

//------------------------------------------------------------------------------

// Node is the highest level interface to a full Tendermint node.
// It includes all configuration information and running services.
type Node struct {
	cmn.BaseService

	// config
	config        *cfg.Config
	genesisDoc    *types.GenesisDoc   // initial validator set
	privValidator types.PrivValidator // local node's validator key

	// network
	privKey          crypto.PrivKeyEd25519   // local node's p2p key
	sw               *p2p.Switch             // p2p connections
	addrBook         *p2p.AddrBook           // known peers
	trustMetricStore *trust.TrustMetricStore // trust metrics for all peers

	// services
	eventBus       *types.EventBus // pub/sub for services
	stateDB        dbm.DB
	blockStore     *bc.BlockStore        // store the blockchain to disk
	bcReactor      *bc.BlockchainReactor // for fast-syncing
	mempoolReactor *mempl.MempoolReactor // for gossipping transactions
	//consensusState   *consensus.ConsensusState   // latest consensus state
	//consensusReactor *consensus.ConsensusReactor // for participating in the consensus
	evidencePool   *evidence.EvidencePool // tracking evidence
	proxyApp       proxy.AppConns         // connection to the application
	rpcListeners   []net.Listener         // rpc servers
	txIndexer      txindex.TxIndexer
	indexerService *txindex.IndexerService
}

// NewNode returns a new, ready to go, Tendermint Node.
func NewNode(config *cfg.Config,
	privValidator types.PrivValidator,
	clientCreator proxy.ClientCreator,
	genesisDocProvider GenesisDocProvider,
	dbProvider DBProvider,
	logger log.Logger) (*Node, error) {

	if len(config.Consensus.Coinbase) == 0 {
		config.Consensus.Coinbase = "0x0000000000000000000000000000000000000000"
	}

	// Get BlockStore
	blockStoreDB, err := dbProvider(&DBContext{"blockstore", config})
	if err != nil {
		return nil, err
	}
	blockStore := bc.NewBlockStore(blockStoreDB)

	// Get State
	stateDB, err := dbProvider(&DBContext{"state", config})
	if err != nil {
		return nil, err
	}

	// Get genesis doc
	// TODO: move to state package?
	genDoc, err := loadGenesisDoc(stateDB)
	if err != nil {
		genDoc, err = genesisDocProvider()
		if err != nil {
			return nil, err
		}

		// save genesis doc to prevent a certain class of user errors (e.g. when it
		// was changed, accidentally or not). Also good for audit trail.
		saveGenesisDoc(stateDB, genDoc)
	}

	state, err := sm.LoadStateFromDBOrGenesisDoc(stateDB, genDoc)
	if err != nil {
		return nil, err
	}

	// Create the proxyApp, which manages connections (consensus, mempool, query)
	// and sync tendermint and the app by performing a handshake
	// and replaying any necessary blocks
	consensusLogger := logger.With("module", "consensus")
	handshaker := consensus.NewHandshaker(stateDB, state, blockStore)
	handshaker.SetLogger(consensusLogger)
	proxyApp := proxy.NewAppConns(clientCreator, handshaker)
	proxyApp.SetLogger(logger.With("module", "proxy"))
	if err := proxyApp.Start(); err != nil {
		return nil, fmt.Errorf("Error starting proxy app connections: %v", err)
	}

	// reload the state (it may have been updated by the handshake)
	state = sm.LoadState(stateDB)

	logger.Info("NewNode, LoadState success", "height", state.LastBlockHeight)

	privKey := privValidator.GetPrikey().Unwrap().(crypto.PrivKeyEd25519)

	//always fast sync
	fastSync := true

	// Make MempoolReactor
	mempoolLogger := logger.With("module", "mempool")
	mempool := mempl.NewMempool(config.Mempool, proxyApp.Mempool(), state.LastBlockHeight)
	mempool.SetLogger(mempoolLogger)
	mempoolReactor := mempl.NewMempoolReactor(config.Mempool, mempool)
	mempoolReactor.SetLogger(mempoolLogger)

	if config.Consensus.WaitForTxs() {
		mempool.EnableTxsAvailable()
	}

	// Make Evidence Reactor
	evidenceDB, err := dbProvider(&DBContext{"evidence", config})
	if err != nil {
		return nil, err
	}
	evidenceLogger := logger.With("module", "evidence")
	evidenceStore := evidence.NewEvidenceStore(evidenceDB)
	evidencePool := evidence.NewEvidencePool(stateDB, evidenceStore)
	evidencePool.SetLogger(evidenceLogger)
	evidenceReactor := evidence.NewEvidenceReactor(evidencePool)
	evidenceReactor.SetLogger(evidenceLogger)

	blockExecLogger := logger.With("module", "state")
	// make block executor for consensus and blockchain reactors to execute blocks
	blockExec := sm.NewBlockExecutor(stateDB, blockExecLogger, proxyApp.Consensus(), mempool, evidencePool)

	// Make BlockchainReactor
	bcReactor := bc.NewBlockchainReactor(state.Copy(), blockExec, blockStore, fastSync)
	bcReactor.SetLogger(logger.With("module", "blockchain"))

	p2pLogger := logger.With("module", "p2p")

	sw := p2p.NewSwitch(config.P2P)
	sw.SetLogger(p2pLogger)
	sw.AddReactor("MEMPOOL", mempoolReactor)
	sw.AddReactor("BLOCKCHAIN", bcReactor)
	sw.AddReactor("EVIDENCE", evidenceReactor)

	blockExec.SetSwitch(sw)

	// Optionally, start the pex reactor
	var addrBook *p2p.AddrBook
	var trustMetricStore *trust.TrustMetricStore
	if config.P2P.PexReactor {
		addrBook = p2p.NewAddrBook(config.P2P.AddrBookFile(), config.P2P.AddrBookStrict)
		addrBook.SetLogger(p2pLogger.With("book", config.P2P.AddrBookFile()))

		// Get the trust metric history data
		trustHistoryDB, err := dbProvider(&DBContext{"trusthistory", config})
		if err != nil {
			return nil, err
		}
		trustMetricStore = trust.NewTrustMetricStore(trustHistoryDB, trust.DefaultConfig())
		trustMetricStore.SetLogger(p2pLogger)

		pexReactor := p2p.NewPEXReactor(addrBook)
		pexReactor.SetLogger(p2pLogger)
		sw.AddReactor("PEX", pexReactor)
	}

	// Filter peers by addr or pubkey with an ABCI query.
	// If the query return code is OK, add peer.
	// XXX: Query format subject to change
	if config.FilterPeers {
		// NOTE: addr is ip:port
		sw.SetAddrFilter(func(addr net.Addr) error {
			resQuery, err := proxyApp.Query().QuerySync(abci.RequestQuery{Path: cmn.Fmt("/p2p/filter/addr/%s", addr.String())})
			if err != nil {
				return err
			}
			if resQuery.IsErr() {
				return resQuery
			}
			return nil
		})
		sw.SetPubKeyFilter(func(pubkey crypto.PubKeyEd25519) error {
			resQuery, err := proxyApp.Query().QuerySync(abci.RequestQuery{Path: cmn.Fmt("/p2p/filter/pubkey/%X", pubkey.Bytes())})
			if err != nil {
				return err
			}
			if resQuery.IsErr() {
				return resQuery
			}
			return nil
		})
	}

	eventBus := types.NewEventBus()
	eventBus.SetLogger(logger.With("module", "events"))

	// Transaction indexing
	var txIndexer txindex.TxIndexer
	switch config.TxIndex.Indexer {
	case "kv":
		store, err := dbProvider(&DBContext{"tx_index", config})
		if err != nil {
			return nil, err
		}
		if config.TxIndex.IndexTags != "" {
			txIndexer = kv.NewTxIndex(store, kv.IndexTags(strings.Split(config.TxIndex.IndexTags, ",")))
		} else if config.TxIndex.IndexAllTags {
			txIndexer = kv.NewTxIndex(store, kv.IndexAllTags())
		} else {
			txIndexer = kv.NewTxIndex(store)
		}
	default:
		txIndexer = &null.TxIndex{}
	}

	indexerService := txindex.NewIndexerService(txIndexer, eventBus)

	// run the profile server
	profileHost := config.ProfListenAddress
	if profileHost != "" {
		go func() {
			logger.Error("Profile server", "err", http.ListenAndServe(profileHost, nil))
		}()
	}

	node := &Node{
		config:        config,
		genesisDoc:    genDoc,
		privValidator: privValidator,

		privKey:          privKey,
		sw:               sw,
		addrBook:         addrBook,
		trustMetricStore: trustMetricStore,

		stateDB:        stateDB,
		blockStore:     blockStore,
		bcReactor:      bcReactor,
		mempoolReactor: mempoolReactor,
		evidencePool:   evidencePool,
		proxyApp:       proxyApp,
		txIndexer:      txIndexer,
		indexerService: indexerService,
		eventBus:       eventBus,
	}
	node.BaseService = *cmn.NewBaseService(logger, "Node", node)
	return node, nil
}

func (n *Node) startPprofService() {
	addr := fmt.Sprintf("%s:%d", n.config.Debug.Pprofaddr, n.config.Debug.Pprofport)
	n.Logger.Info("Start HTTP Pprof server", "addr", addr)
	go func() {
		res := http.ListenAndServe(addr, nil)
		n.Logger.Error("HTTP Pprof server stopped", "result", res)
	}()
}

// OnStart starts the Node. It implements cmn.Service.
func (n *Node) OnStart() error {
	if len(n.config.Debug.CpuProfile) > 0 {
		if err := debug.Handler.StartCPUProfile(n.config.Debug.CpuProfile); err != nil {
			n.Logger.Error("StartCPUProfile", "err", err)
		}
	}
	if len(n.config.Debug.Trace) > 0 {
		if err := debug.Handler.StartGoTrace(n.config.Debug.Trace); err != nil {
			n.Logger.Error("StartGoTrace", "err", err)
		}
	}

	n.Logger.Debug("OnStart", "Pprof", n.config.Debug.Pprof, "Pprofaddr", n.config.Debug.Pprofaddr, "Pprofport", n.config.Debug.Pprofport)
	if n.config.Debug.Pprof {
		n.startPprofService()
	}

	err := n.eventBus.Start()
	if err != nil {
		return err
	}

	// Run the RPC server first
	// so we can eg. receive txs for the first block
	if n.config.RPC.ListenAddress != "" {
		listeners, err := n.startRPC()
		if err != nil {
			return err
		}
		n.rpcListeners = listeners
	}

	// Create & add listener
	if n.config.NodeType != "peer" {
		protocol, address := cmn.ProtocolAndAddress(n.config.P2P.ListenAddress)
		l := p2p.NewDefaultListener(protocol, address, n.config.P2P.SkipUPNP, n.Logger.With("module", "p2p"))
		n.sw.AddListener(l)
	}

	// Start the switch
	n.sw.SetNodeInfo(n.makeNodeInfo())
	n.sw.SetNodePrivKey(n.privKey)
	err = n.sw.Start()
	if err != nil {
		return err
	}

	go n.updateSwitchAddress()
	go n.updateSwichAddressFromBootNodeUrl()

	// start tx indexer
	return n.indexerService.Start()
}

// GetTendermintP2PAddrs return validator p2p addr and port
func (n *Node) GetTendermintP2PAddrs() []string {
	return strings.Split(n.config.P2P.Seeds, ",")
}

func (n *Node) updateSwitchAddress() {
	const retryInterval = 5
	for {
		addrs := n.GetTendermintP2PAddrs()
		if len(addrs) == 0 {
			addrs = n.GetP2PAddrsFromBootNodeUrl()
		}
		if len(addrs) > 0 {
			p2p.AllSeeds = addrs
			if err := n.sw.DialSeeds(n.addrBook, addrs); err != nil {
				n.Logger.Error("updateSwitchAddress: ", "err", err, "addrs", len(addrs))
				continue
			}
			n.Logger.Info("updateSwitchAddress: success", "addrs", len(addrs))
			return
		}
		n.Logger.Error("get p2p addrs faield. get again")
		time.Sleep(retryInterval * time.Second)
	}
}

func (n *Node) updateSwichAddressFromBootNodeUrl() {
	const retryInterval = 5
	for {
		if n.sw.Peers().Size() == 0 {
			addrs := n.GetP2PAddrsFromBootNodeUrl()
			if len(addrs) > 0 {
				p2p.AllSeeds = addrs
				if err := n.sw.DialSeeds(n.addrBook, addrs); err != nil {
					n.Logger.Error("updateSwitchAddress: ", "err", err, "addrs", len(addrs))
				} else {
					n.Logger.Info("updateSwitchAddress: success", "addrs", len(addrs))
				}
			} else {
				n.Logger.Error("get p2p addrs faield. get again")
			}
		}

		time.Sleep(retryInterval * time.Second)
	}
}

type bootNodePeers struct {
	Peers []string `json:"peers"`
}

func (n *Node) GetP2PAddrsFromBootNodeUrl() []string {
	if len(n.config.P2P.BootNodeURL) == 0 {
		n.Logger.Error("config p2p.bootnodeurl is empty")
		return []string{}
	}
	resp, err := http.Get(n.config.P2P.BootNodeURL)
	if err != nil {
		n.Logger.Error("http get request failed.", "url", n.config.P2P.BootNodeURL, "err", err.Error())
		return []string{}
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		n.Logger.Error("ioutil.ReadAll failed.", "err", err.Error())
		return []string{}
	}

	bootNodePeersObj := &bootNodePeers{}
	err = json.Unmarshal(body, bootNodePeersObj)
	if err != nil {
		n.Logger.Error("json unmarshal failed.", "body", string(body), "err", err.Error())
		return []string{}
	}

	return bootNodePeersObj.Peers
}

// OnStop stops the Node. It implements cmn.Service.
func (n *Node) OnStop() {
	n.BaseService.OnStop()

	n.Logger.Info("Stopping Node")
	// TODO: gracefully disconnect from peers.
	n.sw.Stop()

	for _, l := range n.rpcListeners {
		n.Logger.Info("Closing rpc listener", "listener", l)
		if err := l.Close(); err != nil {
			n.Logger.Error("Error closing listener", "listener", l, "err", err)
		}
	}

	n.eventBus.Stop()

	n.indexerService.Stop()
	if len(n.config.Debug.Trace) > 0 {
		debug.Handler.StopGoTrace()
	}
	if len(n.config.Debug.CpuProfile) > 0 {
		debug.Handler.StopCPUProfile()
	}
}

// RunForever waits for an interrupt signal and stops the node.
func (n *Node) RunForever() {
	// Sleep forever and then...
	cmn.TrapSignal(func() {
		n.Stop()
	})
}

// AddListener adds a listener to accept inbound peer connections.
// It should be called before starting the Node.
// The first listener is the primary listener (in NodeInfo)
func (n *Node) AddListener(l p2p.Listener) {
	n.sw.AddListener(l)
}

// ConfigureRPC sets all variables in rpccore so they will serve
// rpc calls from this node
func (n *Node) ConfigureRPC() {
	rpccore.SetStateDB(n.stateDB)
	rpccore.SetBlockStore(n.blockStore)
	rpccore.SetMempool(n.mempoolReactor.Mempool)
	rpccore.SetEvidencePool(n.evidencePool)
	rpccore.SetSwitch(n.sw)
	rpccore.SetPubKey(n.privValidator.GetPubKey())
	rpccore.SetGenesisDoc(n.genesisDoc)
	rpccore.SetAddrBook(n.addrBook)
	rpccore.SetProxyAppQuery(n.proxyApp.Query())
	rpccore.SetTxIndexer(n.txIndexer)
	rpccore.SetEventBus(n.eventBus)
	rpccore.SetLogger(n.Logger.With("module", "rpc"))
}

func (n *Node) startRPC() ([]net.Listener, error) {
	n.ConfigureRPC()
	listenAddrs := strings.Split(n.config.RPC.ListenAddress, ",")

	if n.config.RPC.Unsafe {
		rpccore.AddUnsafeRoutes()
	}

	// we may expose the rpc over both a unix and tcp socket
	listeners := make([]net.Listener, len(listenAddrs))
	for i, listenAddr := range listenAddrs {
		mux := http.NewServeMux()
		rpcLogger := n.Logger.With("module", "rpc-server")
		wm := rpcserver.NewWebsocketManager(rpccore.Routes, rpcserver.EventSubscriber(n.eventBus))
		wm.SetLogger(rpcLogger.With("protocol", "websocket"))
		mux.HandleFunc("/websocket", wm.WebsocketHandler)
		rpcserver.RegisterRPCFuncs(mux, rpccore.Routes, rpcLogger)
		listener, err := rpcserver.StartHTTPServer(listenAddr, mux, rpcLogger)
		if err != nil {
			return nil, err
		}
		listeners[i] = listener
	}

	// we expose a simplified api over grpc for convenience to app devs
	grpcListenAddr := n.config.RPC.GRPCListenAddress
	if grpcListenAddr != "" {
		listener, err := grpccore.StartGRPCServer(grpcListenAddr)
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, listener)
	}

	return listeners, nil
}

// Switch returns the Node's Switch.
func (n *Node) Switch() *p2p.Switch {
	return n.sw
}

// BlockStore returns the Node's BlockStore.
func (n *Node) BlockStore() *bc.BlockStore {
	return n.blockStore
}

// MempoolReactor returns the Node's MempoolReactor.
func (n *Node) MempoolReactor() *mempl.MempoolReactor {
	return n.mempoolReactor
}

// EvidencePool returns the Node's EvidencePool.
func (n *Node) EvidencePool() *evidence.EvidencePool {
	return n.evidencePool
}

// EventBus returns the Node's EventBus.
func (n *Node) EventBus() *types.EventBus {
	return n.eventBus
}

// PrivValidator returns the Node's PrivValidator.
// XXX: for convenience only!
func (n *Node) PrivValidator() types.PrivValidator {
	return n.privValidator
}

// GenesisDoc returns the Node's GenesisDoc.
func (n *Node) GenesisDoc() *types.GenesisDoc {
	return n.genesisDoc
}

// ProxyApp returns the Node's AppConns, representing its connections to the ABCI application.
func (n *Node) ProxyApp() proxy.AppConns {
	return n.proxyApp
}

func (n *Node) makeNodeInfo() *p2p.NodeInfo {
	txIndexerStatus := "on"
	if _, ok := n.txIndexer.(*null.TxIndex); ok {
		txIndexerStatus = "off"
	}
	nodeInfo := &p2p.NodeInfo{
		PubKey:  n.privKey.PubKey().Unwrap().(crypto.PubKeyEd25519),
		Moniker: n.config.Moniker,
		Network: n.genesisDoc.ChainID,
		Version: version.Version,
		Other: []string{
			cmn.Fmt("wire_version=%v", wire.Version),
			cmn.Fmt("p2p_version=%v", p2p.Version),
			cmn.Fmt("consensus_version=%v", consensus.Version),
			cmn.Fmt("rpc_version=%v/%v", rpc.Version, rpccore.Version),
			cmn.Fmt("tx_index=%v", txIndexerStatus),
		},
		Type: n.config.NodeType,
	}

	rpcListenAddr := n.config.RPC.ListenAddress
	nodeInfo.Other = append(nodeInfo.Other, cmn.Fmt("rpc_addr=%v", rpcListenAddr))

	localip := p2p.GetLocalAllAddress()
	_, address := cmn.ProtocolAndAddress(n.config.P2P.ListenAddress)
	lport := strings.Split(address, ":")[1]
	for _, lIp := range localip {
		nodeInfo.LocalAddrs = append(nodeInfo.LocalAddrs, cmn.Fmt("%v:%v", lIp, lport))
	}

	if !n.sw.IsListening() {
		if len(nodeInfo.LocalAddrs) > 0 {
			nodeInfo.ListenAddr = nodeInfo.LocalAddrs[0]
		}
		return nodeInfo
	}

	p2pListener := n.sw.Listeners()[0]
	p2pHost := p2pListener.ExternalAddress().IP.String()
	p2pPort := p2pListener.ExternalAddress().Port
	nodeInfo.ListenAddr = cmn.Fmt("%v:%v", p2pHost, p2pPort)

	return nodeInfo
}

//------------------------------------------------------------------------------

// NodeInfo returns the Node's Info from the Switch.
func (n *Node) NodeInfo() *p2p.NodeInfo {
	return n.sw.NodeInfo()
}

// DialSeeds dials the given seeds on the Switch.
func (n *Node) DialSeeds(seeds []string) error {
	return n.sw.DialSeeds(n.addrBook, seeds)
}

//------------------------------------------------------------------------------

var (
	genesisDocKey = []byte("genesisDoc")
)

// panics if failed to unmarshal bytes
func loadGenesisDoc(db dbm.DB) (*types.GenesisDoc, error) {
	bytes := db.Get(genesisDocKey)
	if len(bytes) == 0 {
		return nil, errors.New("Genesis doc not found")
	} else {
		var genDoc *types.GenesisDoc
		err := json.Unmarshal(bytes, &genDoc)
		if err != nil {
			cmn.PanicCrisis(fmt.Sprintf("Failed to load genesis doc due to unmarshaling error: %v (bytes: %X)", err, bytes))
		}
		return genDoc, nil
	}
}

// panics if failed to marshal the given genesis document
func saveGenesisDoc(db dbm.DB, genDoc *types.GenesisDoc) {
	bytes, err := json.Marshal(genDoc)
	if err != nil {
		cmn.PanicCrisis(fmt.Sprintf("Failed to save genesis doc due to marshaling error: %v", err))
	}
	db.SetSync(genesisDocKey, bytes)
}
