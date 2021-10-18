package metrics

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/pkg/errors"
	"github.com/protolambda/rumor/metrics/utils"
	"github.com/protolambda/rumor/p2p/gossip/database"
	log "github.com/sirupsen/logrus"
)

type ErrorHandling func(*Peer)

type PeerStore struct {
	PeerStore         PeerStoreStorage
	MessageDatabase   *database.MessageDatabase // TODO: Discuss
	StartTime         time.Time
	PeerstoreIterTime time.Duration
	MsgNotChannels    map[string](chan bool) // TODO: Unused?
}

func NewPeerStore(dbtype string, path string) PeerStore {
	var db PeerStoreStorage
	switch dbtype {
	case "bold":
		if len(path) <= 0 {
			path = default_db_path
		}
		db = NewBoltPeerDB(path)
	case "memory":
		db = NewMemoryDB()
	default:
		if len(path) <= 0 {
			path = default_db_path
		}
		db = NewBoltPeerDB(path)
	}
	ps := PeerStore{
		PeerStore:      db,
		StartTime:      time.Now(),
		MsgNotChannels: make(map[string](chan bool)),
	}
	return ps
}

func (c *PeerStore) ImportPeerStoreMetrics(importFolder string) error {
	// TODO: Load to memory an existing csv
	// Perhaps not needed since we are migrating to a database
	// See: https://github.com/migalabs/armiarma/pull/18

	return nil
}

// Function that resets to 0 the connections/disconnections, and message counters
// this way the Ram Usage gets limited (up to ~10k nodes for a 12h-24h )
// NOTE: Keep in mind that the peers that we ended up connected to, will experience a weid connection time
// TODO: Fix peers that stayed connected to the tool
func (c *PeerStore) ResetDynamicMetrics() {
	log.Info("Reseting Dynamic Metrics in Peer")

	// Iterate throught the peers in the metrics, restarting connection events and messages
	c.PeerStore.Range(func(key string, value Peer) bool {
		value.ResetDynamicMetrics()
		c.PeerStore.Store(key, value)
		return true
	})
	log.Info("Finished Reseting Dynamic Metrics")
}

// Function that adds a notification channel to the message gossip topic
func (c *PeerStore) AddNotChannel(topicName string) {
	c.MsgNotChannels[topicName] = make(chan bool, 100)
}

// Updates the peer without overwritting all its content
func (c *PeerStore) StoreOrUpdatePeer(peer Peer) {
	// TODO: We could also store the old data if there was a change. For example
	// if a given client upgrated it version. Use oldData
	// See: https://github.com/migalabs/armiarma/issues/17
	// Currently just overwritting what was before
	oldPeer, err := c.GetPeerData(peer.PeerId)
	// if error means not found, just store it
	if err != nil {
		c.PeerStore.Store(peer.PeerId, peer)
	} else {
		// Fetch the new info of a peer directly from the new peer struct
		oldPeer.FetchPeerInfoFromPeer(peer)
		c.PeerStore.Store(peer.PeerId, oldPeer)
	}
	// Force Garbage collector
	runtime.GC()
}

// Stores exact peer, overwritting the existing peer. Use only internally
func (c *PeerStore) StorePeer(peer Peer) {
	c.PeerStore.Store(peer.PeerId, peer)
}

// Get peer data
func (c *PeerStore) GetPeerData(peerId string) (Peer, error) {
	peerData, ok := c.PeerStore.Load(peerId)
	if !ok {
		return Peer{}, errors.New("could not find peer in peerstore: " + peerId)
	}
	return peerData, nil
}

/// AddNewAttempts adds the resuts of a negative new attempt over an existing peer
// increasing the attempt counter and the respective fields
func (c *PeerStore) AddNewNegConnectionAttempt(id string, rec_err string, fn ErrorHandling) error {
	p, err := c.GetPeerData(id)
	if err != nil { // the peer was already in the sync.Map return true
		return fmt.Errorf("Not peer found with that ID %s", id)
	}
	// Update the counter and connection status
	p.Attempts += 1
	if !p.Attempted {
		p.Attempted = true
		p.Error = utils.FilterError(rec_err)

	}
	// Handle each of the different error types as defined currently at pruneconnect.go
	fn(&p)

	// Store the new struct in the sync.Map
	c.StorePeer(p)
	return nil
}

// AddNewPosConnectionAttempt adds the resuts of a possitive new attempt over an existing peer
// increasing the attempt counter and the respective fields
func (c *PeerStore) AddNewPosConnectionAttempt(id string) error {
	p, err := c.GetPeerData(id)
	if err != nil { // the peer was already in the sync.Map return true
		return fmt.Errorf("Not peer found with that ID %s", id)
	}
	// Update the counter and connection status
	p.Attempts += 1
	if !p.Attempted {
		p.Attempted = true

	}
	p.Succeed = true
	p.Error = "None"
	// clean the Negative connection Attempt list
	p.AddPositiveConnAttempt()
	// Store the new struct in the sync.Map
	c.StorePeer(p)
	return nil
}

// Add a connection Event to the given peer
func (c *PeerStore) ConnectionEvent(peerId string, direction string) error {
	peer, err := c.GetPeerData(peerId)
	if err != nil {
		return errors.New("could not add connection event, peer is not in the list: " + peerId)
	}
	peer.ConnectionEvent(direction, time.Now())
	c.StorePeer(peer)
	return nil
}

// Add a connection Event to the given peer
func (c *PeerStore) DisconnectionEvent(peerId string) error {
	peer, err := c.GetPeerData(peerId)
	if err != nil {
		return errors.New("could not add disconnection event, peer is not in the list: " + peerId)
	}
	peer.DisconnectionEvent(time.Now())
	c.StorePeer(peer)
	return nil
}

// Add a connection Event to the given peer
func (c *PeerStore) MetadataEvent(peerId string, success bool) error {
	peer, err := c.GetPeerData(peerId)
	if err != nil {
		return errors.New("could not add metadata event, peer is not in the list: " + peerId)
	}
	peer.MetadataRequest = true
	if success {
		peer.MetadataSucceed = true
	}
	c.StorePeer(peer)
	return nil
}

// AddNewAttempts adds the resuts of a new attempt over an existing peer
// increasing the attempt counter and the respective fields
func (c *PeerStore) ConnectionAttemptEvent(peerId string, succeed bool, conErr string) error {
	peer, err := c.GetPeerData(peerId)
	if err != nil {
		return errors.New("could not add connection attempt, peer is not in the list: " + peerId)
	}
	peer.ConnectionAttemptEvent(succeed, conErr)
	c.StorePeer(peer)
	return nil
}

// Function that Manages the metrics updates for the incoming messages
// TODO: Rename to AddNewMessageEvent or something like that
func (c *PeerStore) MessageEvent(peerId string, topicName string) error {
	peer, err := c.GetPeerData(peerId)
	if err != nil {
		return errors.New("could not add message event, peer is not in the list: " + peerId)
	}
	peer.MessageEvent(topicName, time.Now())
	c.StorePeer(peer)
	return nil
}

// Get a map with the errors we got when connecting and their amount
func (gm *PeerStore) GetErrorCounter() map[string]uint64 {
	errorsAndAmount := make(map[string]uint64)
	gm.PeerStore.Range(func(key string, value Peer) bool {
		errorsAndAmount[value.Error]++
		return true
	})

	return errorsAndAmount
}

// Update the last iteration throught whole PeerStore
func (c *PeerStore) NewPeerstoreIteration(t time.Duration) {
	c.PeerstoreIterTime = t
}

// Exports to a csv, useful for debug
func (c *PeerStore) ExportToCSV(filePath string) error {
	log.Info("Exporting metrics to csv: ", filePath)
	csvFile, err := os.Create(filePath)
	if err != nil {
		return errors.Wrap(err, "error opening the file "+filePath)
	}
	defer csvFile.Close()

	// First raw of the file will be the Titles of the columns
	_, err = csvFile.WriteString("Peer Id,Node Id,User Agent,Client,Version,Pubkey,Address,Ip,Country,City,Request Metadata,Success Metadata,Attempted,Succeed,ConnStablished,IsConnected,Attempts,Error,Latency,Connections,Disconnections,Connected Time,Beacon Blocks,Beacon Aggregations,Voluntary Exits,Proposer Slashings,Attester Slashings,Total Messages\n")
	if err != nil {
		errors.Wrap(err, "error while writing the titles on the csv "+filePath)
	}

	err = nil
	c.PeerStore.Range(func(key string, value Peer) bool {
		_, err = csvFile.WriteString(value.ToCsvLine())
		return true
	})

	if err != nil {
		return errors.Wrap(err, "could not export peer metrics")
	}
	// Force Garbage collector
	runtime.GC()
	return nil
}
