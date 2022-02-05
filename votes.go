package pvm

import (
	"context"
	"encoding/json"
	"github.com/microcosm-cc/bluemonday"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/types"
	"log"
	"math"
	"time"
)

type VoteState struct {
	Index int32
	Type  string
	Time  time.Time
	Height int64
}

func Votes(ctx context.Context, client *rpchttp.HTTP, state chan *VoteState) {
	event, err := client.Subscribe(ctx, "pvmon-votes", "tm.event = 'Vote'")
	if err != nil {
		log.Println(err)
		return
	}
	defer client.Unsubscribe(context.Background(), "pvmon-votes", "tm.event = 'Vote'")

	for {
		select {
		case e := <-event:
			v := e.Data.(types.EventDataVote).Vote
			if v.Type == 1 {
				state <- &VoteState{
					Index: v.ValidatorIndex,
					Type:  v.Type.String(),
					Time:  v.Timestamp,
					Height: v.Height,
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

type NewRound struct {
	Height int64
	Index  int32
}

func Round(ctx context.Context, client *rpchttp.HTTP, rounds chan *NewRound) {
	defer log.Println("Not watching rounds")
	event, err := client.Subscribe(ctx, "pvmon-round", "tm.event = 'NewRound'")
	if err != nil {
		log.Println(err)
		return
	}
	defer client.Unsubscribe(context.Background(), "pvmon-round", "tm.event = 'NewRound'")

	for {
		select {
		case e := <-event:
			v, ok := e.Data.(types.EventDataNewRound)
			if !ok {
				log.Println("could not parse round message")
				continue
			}
			rounds <- &NewRound{
				Height: v.Height,
				Index:  v.Proposer.Index,
			}
		case <-ctx.Done():
			return
		}
	}
}

func Header(ctx context.Context, client *rpchttp.HTTP, last chan int64) {
	event, err := client.Subscribe(ctx, "pvmon-header", "tm.event = 'NewBlockHeader'")
	if err != nil {
		log.Println(err)
		return
	}
	defer client.Unsubscribe(context.Background(),"pvmon-header", "tm.event = 'NewBlockHeader'")

	for {
		select {
		case e := <-event:
			v := e.Data.(types.EventDataNewBlockHeader)
			last <- v.Header.Height
		case <-ctx.Done():
			return
		}
	}
}

type NewRoundMsg struct {
	Type         string `json:"type"`
	Proposer     string `json:"proposer"`
	ProposerOper string `json:"proposer_oper"`
	Height       int64  `json:"height"`
	TimeStamp    int64  `json:"time_stamp"`
}

type PreVoteMsg struct {
	Type     string  `json:"type"`
	Moniker  string  `json:"moniker"`
	ValOper  string  `json:"valoper"`
	Weight   float64 `json:"weight"`
	OffsetMs int64   `json:"offset_ms"`
	Height   int64   `json:"height"`
}

type ProgressMsg struct {
	Type string `json:"type"`
	Pct float64 `json:"pct"`
	TimeStamp int64 `json:"time_stamp"`
}

type CurrentState struct {
	Round *NewRoundMsg `json:"round"`
	PreVotes []*PreVoteMsg `json:"pre_votes"`
	Progress *ProgressMsg `json:"progress"`
}

var State *CurrentState

func WatchPrevotes(rpc, rest string, rounds, updates, progress chan []byte) {

	abort, cancel := context.WithCancel(context.Background())
	defer cancel()

	currentVals := make([]*Val, 0)
	valUpdates := make(chan []*Val)
	go func() {
		for {
			select {
			case currentVals = <-valUpdates:
			case <-abort.Done():
				return
			}

		}
	}()
	go func(){
		Vals(abort, rest, valUpdates)
		cancel()
	}()

	time.Sleep(6 * time.Second) // ensure we have a valset before continuing, lazy lazy using sleep :P

	client, _ := rpchttp.New(rpc, "/websocket")
	err := client.Start()
	if err != nil {
		log.Println(err)
		return
	}
	defer client.Stop()

	currentRound := &NewRound{}
	newRound := make(chan *NewRound)
	var lastTS time.Time
	var pct float64
	bm := bluemonday.StrictPolicy()
	go func() {
		for {
			select {
			case currentRound = <-newRound:
				lastTS = time.Now().UTC()
				pct = 0
				State.PreVotes = make([]*PreVoteMsg, 0)
				State.Progress = &ProgressMsg{
					Type:      "pct",
					Pct:       0,
					TimeStamp: time.Now().UTC().Unix(),
				}
				if pJson, e := json.Marshal(State.Progress); e == nil {
					progress <- pJson
				}
				if int32(len(currentVals)) < currentRound.Index || currentVals == nil {
					log.Println("not ready")
					continue
				}
				State.Round = &NewRoundMsg{
					Type:         "round",
					Proposer:     bm.Sanitize(currentVals[currentRound.Index].Moniker),
					ProposerOper: currentVals[currentRound.Index].Valoper,
					Height:       currentRound.Height,
					TimeStamp:    lastTS.UTC().Unix(),
				}
				roundJson, e := json.Marshal(State.Round)
				if e != nil {
					log.Println(e)
					continue
				}
				rounds <- roundJson
			case <-abort.Done():
				return
			}
		}
	}()
	go func() {
		Round(abort, client, newRound)
		cancel()
	}()

	var lastHeight int64
	headerHeight := make(chan int64)
	go func() {
		for {
			lastHeight = <-headerHeight
		}
	}()
	go func(){
		Header(abort, client, headerHeight)
		cancel()
	}()

	go func() {
		tick := time.NewTicker(500*time.Millisecond)
		for {
			select {
			case <-tick.C:
				if pct > 100 {
					continue
				}
				State.Progress = &ProgressMsg{
					Type:      "pct",
					Pct:       math.Round(pct * 100)/100,
					TimeStamp: time.Now().UTC().Unix(),
				}
				if pJson, e := json.Marshal(State.Progress); e == nil {
					progress <- pJson
				}
			case <-abort.Done():
				return
			}
		}
	}()

	votes := make(chan *VoteState)
	go func(){
		Votes(abort, client, votes)
		cancel()
	}()

	for {
		select {
		case v := <-votes:
			if len(currentVals) == 0 || int32(len(currentVals)) < v.Index || v.Height != lastHeight+1 {
				continue
			}
			//fmt.Printf("%60s: %3.2f%% %s\n", currentVals[int(v.Index)].Moniker, 100*currentVals[int(v.Index)].Weight, v.Time.Sub(lastTS).String())
			newVote := &PreVoteMsg{
				Type:     "prevote",
				Moniker:  currentVals[int(v.Index)].Moniker,
				ValOper:  currentVals[int(v.Index)].Valoper,
				Weight:   float64(math.Floor(100000*currentVals[int(v.Index)].Weight)) / 1000, // three digits of precision, rounded down.
				OffsetMs: v.Time.Sub(lastTS).Milliseconds(),
				Height:   v.Height,
			}
			State.PreVotes = append(State.PreVotes, newVote)
			j, e := json.Marshal(newVote)
			pct += float64(math.Floor(100000*currentVals[int(v.Index)].Weight)) / 1000
			if e != nil {
				log.Println(e)
				continue
			}
			updates <- j
		case <-abort.Done():
			cancel()
			return
		}
	}

}
