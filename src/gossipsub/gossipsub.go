/**
This packages describes the implementation of the GossipSub
protocol.
It also provides all needed functions and methods todeploy and interact
with a GossipSub service


*/

package gossipsub

import (
	"context"
	"encoding/base64"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/migalabs/armiarma/src/db"
	"github.com/migalabs/armiarma/src/hosts"
	"github.com/migalabs/armiarma/src/info"
	"github.com/minio/sha256-simd"
	"github.com/sirupsen/logrus"
)

var (
	ModuleName = "GOSSIP-SUB"
	Log        = logrus.WithField(
		"module", ModuleName,
	)
)

// GossipSub
// sumarizes the control fields necesary to manage and
// govern the GossipSub internal service.
type GossipSub struct {
	ctx           context.Context
	cancel        context.CancelFunc
	InfoObj       *info.InfoData
	BasicHost     *hosts.BasicLibp2pHost
	PeerStore     *db.PeerStore
	PubsubService *pubsub.PubSub
	// map where the key are the topic names in string, and the values are the TopicSubscription
	TopicArray     map[string]*TopicSubscription
	MessageMetrics *MessageMetrics
}

// NewEmptyGossipSub:
// Sumarizes the control fields necesary to manage and
// govern over a joined and subscribed topic
// @return: gossipsub struct
func NewEmptyGossipSub() *GossipSub {
	return &GossipSub{}
}

// NewGossipSub:
// Sumarizes the control fields necesary to manage and
// govern over a joined and subscribed topic.
// @param ctx: parent context for the gossip service.
// @param h: the libp2p.PubSub topic of the joined topic.
// @param peerstore: the peerstore where to sotre the data.
// @param stdOpts: list of options to generate the base of the gossipsub service.
// @return: pointer to GossipSub struct.
func NewGossipSub(ctx context.Context, h *hosts.BasicLibp2pHost, peerstore *db.PeerStore) *GossipSub {
	mainCtx, cancel := context.WithCancel(ctx)

	// define gossipsub option
	// Signature is not used in Eth2, therefore it is needed
	// to specify this options to false
	// Otherwise, messages are discarded
	psOptions := []pubsub.Option{
		pubsub.WithMessageSigning(false),
		pubsub.WithStrictSignatureVerification(false),
		pubsub.WithMessageIdFn(MsgIDFunction),
	}
	ps, err := pubsub.NewGossipSub(mainCtx, h.Host(), psOptions...)
	if err != nil {
		Log.Panic(err)
	}
	msgMetrics := NewMessageMetrics()
	// return the GossipSub object
	return &GossipSub{
		ctx:            mainCtx,
		cancel:         cancel,
		InfoObj:        h.GetInfoObj(),
		BasicHost:      h,
		PeerStore:      peerstore,
		PubsubService:  ps,
		TopicArray:     make(map[string]*TopicSubscription),
		MessageMetrics: &msgMetrics,
	}
}

// WithMessageIdFn is an option to customize the way a message ID is computed for a pubsub message
func MsgIDFunction(pmsg *pubsub_pb.Message) string {
	h := sha256.New()
	// never errors, see crypto/sha256 Go doc

	_, _ = h.Write(pmsg.Data)
	id := h.Sum(nil)
	return base64.URLEncoding.EncodeToString(id)
}

// JoinAndSubscribe:
// This method allows the GossipSub service to join and
// subscribe to a topic.
// @param topicName: name of the topic to subscribe.
// @return: pointer to GossipSub struct.
func (gs *GossipSub) JoinAndSubscribe(topicName string) {
	// Join topic
	topic, err := gs.PubsubService.Join(topicName)
	if err != nil {
		Log.Errorf("Could not join topic: %s", topicName)
		Log.Errorf(err.Error())
	}
	// Subscribe to the topic
	sub, err := topic.Subscribe()
	if err != nil {
		Log.Errorf("Could not subscribe to topic: %s", topicName)
		Log.Errorf(err.Error())
	}
	// Add the topic to the metrics list
	_ = gs.MessageMetrics.NewTopic(topicName)

	new_topic_handler := NewTopicSubscription(gs.ctx, topic, *sub, gs.MessageMetrics)
	// Add the new Topic to the list of supported/subscribed topics in GossipSub
	gs.TopicArray[topicName] = new_topic_handler

	go gs.TopicArray[topicName].MessageReadingLoop(gs.BasicHost.Host(), gs.PeerStore)
}

func (gs *GossipSub) Close() {
	Log.Info("gossipsub close has been detected, closing dependant go-routines")
	gs.cancel()
}

func (gs *GossipSub) Ctx() context.Context {
	return gs.ctx
}

func (gs *GossipSub) GetCtxCancel() context.CancelFunc {
	return gs.cancel
}
