package pvm

import (
	"context"
	"encoding/json"
	"fmt"
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

func Votes(client *rpchttp.HTTP, state chan *VoteState) {
	event, err := client.Subscribe(context.Background(), "pvmon-votes", "tm.event = 'Vote'")
	if err != nil {
		panic(err)
	}

	for e := range event {
		v := e.Data.(types.EventDataVote).Vote
		if v.Type == 1 {
			state <- &VoteState{
				Index: v.ValidatorIndex,
				Type:  v.Type.String(),
				Time:  v.Timestamp,
				Height: v.Height,
			}
		}
	}
}

type NewRound struct {
	Height int64
	Index  int32
}

func Round(client *rpchttp.HTTP, rounds chan *NewRound) {
	event, err := client.Subscribe(context.Background(), "pvmon-round", "tm.event = 'NewRound'")
	if err != nil {
		panic(err)
	}

	for e := range event {
		v := e.Data.(types.EventDataNewRound)
		rounds <- &NewRound{
			Height: v.Height,
			Index:  v.Proposer.Index,
		}
	}
}

func Header(client *rpchttp.HTTP, last chan int64) {
	event, err := client.Subscribe(context.Background(), "pvmon-header", "tm.event = 'NewBlockHeader'")
	if err != nil {
		panic(err)
	}

	for e := range event {
		v := e.Data.(types.EventDataNewBlockHeader)
		last <- v.Header.Height
	}
}

func WatchPrevotes(rpc, rest string, updates chan []byte) {
	type newRoundMsg struct {
		Type         string `json:"type"`
		Proposer     string `json:"proposer"`
		ProposerOper string `json:"proposer_oper"`
		Height       int64  `json:"height"`
		TimeStamp    int64  `json:"time_stamp"`
	}

	type preVoteMsg struct {
		Type     string  `json:"type"`
		Moniker  string  `json:"moniker"`
		ValOper  string  `json:"valoper"`
		Weight   float64 `json:"weight"`
		OffsetMs int64   `json:"offset_ms"`
		Height   int64   `json:"height"`
	}

	type pctMsg struct {
		Height int64 `json:"height"`
		Pct float64 `json:"pct"`
	}

	currentVals := make([]*Val, 0)
	valUpdates := make(chan []*Val)
	go Vals(rest, valUpdates)
	go func() {
		for {
			currentVals = <-valUpdates
		}
	}()
	time.Sleep(6 * time.Second) // ensure we have a valset before continuing, lazy lazy using sleep :P

	client, _ := rpchttp.New(rpc, "/websocket")
	err := client.Start()
	if err != nil {
		panic(err)
	}
	defer client.Stop()

	currentRound := &NewRound{}
	newRound := make(chan *NewRound)
	var lastTS time.Time
	var pct float64
	go Round(client, newRound)
	go func() {
		for {
			currentRound = <-newRound
			lastTS = time.Now().UTC()
			pct = 0
			updates <- []byte(fmt.Sprintf(`{"type": "pct", "pct": %.2f, "time_stamp": %d}`, pct, time.Now().UTC().Unix()))
			//fmt.Println("starting new round:", currentRound.Height)
			if int32(len(currentVals)) < currentRound.Index {
				log.Println("not ready")
				continue
			}
			//if int32(len(currentVals)) >= currentRound.Index {
			//	fmt.Println("new proposer:", currentVals[currentRound.Index].Moniker)
			//}
			j, e := json.Marshal(newRoundMsg{
				Type:         "round",
				Proposer:     currentVals[currentRound.Index].Moniker,
				ProposerOper: currentVals[currentRound.Index].Valoper,
				Height:       currentRound.Height,
				TimeStamp:    lastTS.UTC().Unix(),
			})
			if e != nil {
				log.Println(e)
				continue
			}
			updates <- j
		}
	}()

	var lastHeight int64
	headerHeight := make(chan int64)
	go Header(client, headerHeight)
	go func() {
		for {
			lastHeight = <-headerHeight
		}
	}()

	votes := make(chan *VoteState)
	go Votes(client, votes)

	go func() {
		for {
			time.Sleep(time.Second)
			if pct > 100 {
				continue
			}
			updates <- []byte(fmt.Sprintf(`{"type": "pct", "pct": %.2f, "time_stamp": %d}`, pct, time.Now().UTC().Unix()))
		}
	}()

	for v := range votes {
		if len(currentVals) == 0 || int32(len(currentVals)) < v.Index || v.Height != lastHeight+1 {
			continue
		}
		//fmt.Printf("%60s: %3.2f%% %s\n", currentVals[int(v.Index)].Moniker, 100*currentVals[int(v.Index)].Weight, v.Time.Sub(lastTS).String())
		j, e := json.Marshal(preVoteMsg{
			Type:     "prevote",
			Moniker:  currentVals[int(v.Index)].Moniker,
			ValOper:  currentVals[int(v.Index)].Valoper,
			Weight:   float64(math.Floor(100000*currentVals[int(v.Index)].Weight)) / 1000, // three digits of precision, rounded down.
			OffsetMs: v.Time.Sub(lastTS).Milliseconds(),
			Height:   v.Height,
		})
		pct += float64(math.Floor(100000*currentVals[int(v.Index)].Weight)) / 1000
		if e != nil {
			log.Println(e)
			continue
		}
		updates <- j
	}
}
