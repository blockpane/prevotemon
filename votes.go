package pvm

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/microcosm-cc/bluemonday"
	constypes "github.com/tendermint/tendermint/consensus/types"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/types"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

type VoteState struct {
	Index  int32
	Type   string
	Time   time.Time
	Height int64
}

type FinalState struct {
	Type     string `json:"type"`
	Votes    []*PreVoteMsg
	Height   int64   `json:"height"`
	Proposer string  `json:"proposer"`
	Percent  float64 `json:"percent"`
}

var currentVals = make([]*Val, 0)

type simple constypes.RoundStateSimple

func (s simple) height() int64 {
	h, _ := strconv.Atoi(strings.Split(s.HeightRoundStep, "/")[0])
	return int64(h)
}

func (s simple) round() int {
	h, _ := strconv.Atoi(strings.Split(s.HeightRoundStep, "/")[1])
	return h
}

func (s simple) prevotes() ([]time.Time, float64) {
	vs := make([]votesString, 0)
	err := json.Unmarshal(s.Votes, &vs)
	if err != nil {
		log.Println(err)
		return nil, 0
	}
	if vs == nil || len(vs) == 0 {
		log.Println("voteset was invalid")
		return nil, 0
	}
	times := gettimes(vs[s.round()].Prevotes)
	return times, vs[s.round()].percent()
}

func (s simple) proposerIndex() int32 {
	vs := make([]votesString, 0)
	err := json.Unmarshal(s.Votes, &vs)
	if err != nil {
		log.Println(err)
		return 0
	}
	if vs == nil || len(vs) == 0 {
		log.Println("voteset was invalid")
		return 0
	}
	return int32(vs[s.round()].Proposer.Index)
}

func gettimes(s []string) []time.Time {
	times := make([]time.Time, len(s))
	for i, v := range s {
		if v == "nil-Vote" {
			continue
		}
		split := strings.Split(v, "@ ")
		if len(split) != 2 {
			continue
		}
		t, _ := time.Parse(time.RFC3339Nano, strings.TrimRight(split[1], `}`))
		times[i] = t.UTC()
	}
	return times
}

type votesString struct {
	Round            int32    `json:"round"`
	Prevotes         []string `json:"prevotes"`
	PrevotesBitArray string   `json:"prevotes_bit_array"`
	Proposer         struct {
		Index int `json:"index"`
	} `json:"proposer"`
}

func (vs votesString) percent() float64 {
	split := strings.Split(vs.PrevotesBitArray, "= ")
	if len(split) != 2 {
		return 0.0
	}
	f, _ := strconv.ParseFloat(split[1], 64)
	return math.Round(f*10000) / 100
}

var Percentage float64
var deDup = make([]bool, 200)
var lastTS = time.Now().UTC()
var nextTS = time.Now().UTC()
var stateHeight int64

func Votes(ctx context.Context, client *rpchttp.HTTP, state chan *VoteState, round chan *NewRound) {
	tick := time.NewTicker(250 * time.Millisecond)
	var previousHeight int64
	var sendNewRound bool
	var busy bool
	for {
		select {
		case <-tick.C:
			func() {
				if busy {
					return
				}
				busy = true
				defer func() {
					busy = false
				}()
				timeout, cnl := context.WithTimeout(context.Background(), time.Second)
				defer cnl()
				resp, err := client.ConsensusState(timeout)
				if err != nil {
					log.Println(err)
					return
				}
				roundState := &simple{}
				err = json.Unmarshal(resp.RoundState, roundState)
				if err != nil {
					log.Println(err)
					return
				}
				votes, _ := roundState.prevotes()
				if votes == nil {
					return
				}
				stateHeight = roundState.height()
				nextTS = roundState.StartTime
				hits := 0

				send := func(votes []time.Time, height int64) {
					for i := range votes {
						if deDup[i] || votes[i].IsZero() {
							continue
						}
						state <- &VoteState{
							Index:  int32(i),
							Type:   "prevote",
							Time:   votes[i],
							Height: height,
						}
						deDup[i] = true
						hits += 1
					}
				}

				if stateHeight > previousHeight {
					dumped := make(map[string]interface{})
					heightStr := strconv.FormatInt(stateHeight-1, 10)
					dumped["height"] = heightStr
					func() {
						for dumped["height"] != heightStr {
							dump, e := client.DumpConsensusState(timeout)
							if e != nil {
								log.Println(e)
								return
							}
							e = json.Unmarshal(dump.RoundState, &dumped)
							if e != nil {
								log.Println(e)
								return
							}
							if h, ok := dumped["height"].(string); ok && h != heightStr {
								if lastCommit, ok := dumped["last_commit"].(map[string]interface{}); ok {
									if finalTimes, ok := lastCommit["votes"].([]string); ok {
										finalCommits := gettimes(finalTimes)
										send(finalCommits, stateHeight-1)
									}
								}

								return
							} else {
								dumped["height"] = heightStr
							}
						}
					}()
					previousHeight = stateHeight
					sendNewRound = true
					hits = 0
				} else if stateHeight < previousHeight {

					return
				}
				send(votes, stateHeight)

				if sendNewRound && hits > 0 && State.Round != nil && stateHeight > State.Round.Height {
					sendNewRound = false
					deDup = make([]bool, 200)
					round <- &NewRound{
						Height: stateHeight,
						Index:  roundState.proposerIndex(),
					}
				}

			}()
		case <-ctx.Done():
			return
		}
	}
}

func VoteStream(ctx context.Context, client *rpchttp.HTTP, state chan *VoteState) {
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
			if v.Type == 1 && !deDup[v.ValidatorIndex] {
				state <- &VoteState{
					Index: v.ValidatorIndex,
					Type:  v.Type.String(),
					//Time:   v.Timestamp,
					Time:   time.Now().UTC(),
					Height: v.Height,
				}
			}
			deDup[v.ValidatorIndex] = true
		case <-ctx.Done():
			return
		}
	}
}

type NewRound struct {
	Height int64
	Index  int32
	Start  time.Time
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
			if State.Round != nil && v.Height == State.Round.Height {
				continue
			}
			deDup = make([]bool, 200)
			rounds <- &NewRound{
				Height: v.Height,
				Index:  v.Proposer.Index,
			}
		case <-ctx.Done():
			return
		}
	}
}

type finalProposer struct {
	sync.Mutex
	Block map[int64][]string
}

var finalProposers = finalProposer{
	Block: make(map[int64][]string),
}

func (fp *finalProposer) add(height int64, moniker, valoper string) {
	fp.Lock()
	defer fp.Unlock()
	fp.Block[height] = []string{moniker, valoper}
	for k := range fp.Block {
		if k < height-10 {
			delete(fp.Block, k)
		}
	}
}

func (fp *finalProposer) get(height int64) (moniker, valoper string) {
	fp.Lock()
	defer fp.Unlock()
	p := fp.Block[height]
	if len(p) == 0 {
		return "", ""
	}
	return p[0], p[1]
}

//func newHeader(ctx context.Context, client *rpchttp.HTTP, last chan int64) {
func newHeader(ctx context.Context, client *rpchttp.HTTP) {
	event, err := client.Subscribe(ctx, "pvmon-header", "tm.event = 'NewBlockHeader'")
	if err != nil {
		log.Println(err)
		return
	}
	defer client.Unsubscribe(context.Background(), "pvmon-header", "tm.event = 'NewBlockHeader'")

	for {
		if len(currentVals) == 0 {
			time.Sleep(time.Second)
			continue
		}
		break
	}

	bm := bluemonday.StrictPolicy()
	for {
		select {
		case e := <-event:
			if v, ok := e.Data.(types.EventDataNewBlockHeader); ok {
				for i := range currentVals {
					valAddr, bad := pubToHexAddr(currentVals[i].pubkey)
					if bad != nil {
						log.Println(bad)
						continue
					}
					if valAddr == v.Header.ProposerAddress.String() {
						finalProposers.add(v.Header.Height, bm.Sanitize(currentVals[i].Moniker), currentVals[i].Valoper)
					}
				}
			}
			//last <- v.Header.Height
		case <-ctx.Done():
			return
		}
	}
}

func pubToHexAddr(pub string) (string, error) {
	b := make([]byte, 32)
	_, err := base64.StdEncoding.Decode(b, []byte(pub))
	if err != nil {
		return "", err
	}
	sha := sha256.New()
	sha.Write(b)
	return strings.ToUpper(hex.EncodeToString(sha.Sum(nil)[:20])), nil
}

type NewRoundMsg struct {
	Type            string `json:"type"`
	Proposer        string `json:"proposer"`
	ProposerOper    string `json:"proposer_oper"`
	Height          int64  `json:"height"`
	TimeStamp       int64  `json:"time_stamp"`
	TimeOutProposer string `json:"time_out_proposer"`
}

type PreVoteMsg struct {
	Type     string  `json:"type"`
	Moniker  string  `json:"moniker"`
	ValOper  string  `json:"valoper"`
	Weight   float64 `json:"weight"`
	OffsetMs int64   `json:"offset_ms"`
	Height   int64   `json:"height"`
	Proposer bool    `json:"proposer"`
}

type ProgressMsg struct {
	Type      string  `json:"type"`
	Pct       float64 `json:"pct"`
	TimeStamp int64   `json:"time_stamp"`
}

type CurrentState struct {
	Round    *NewRoundMsg  `json:"round"`
	PreVotes []*PreVoteMsg `json:"pre_votes"`
	Progress *ProgressMsg  `json:"progress"`
}

type NewProposer struct {
	Type            string `json:"type"`
	Proposer        string `json:"proposer"`
	ProposerOper    string `json:"proposer_oper"`
	TimeOutProposer string `json:"time_out_proposer"`
}

var State *CurrentState

func WatchPrevotes(rpc, rest string, rounds, updates, progress chan []byte) {

	abort, cancel := context.WithCancel(context.Background())
	defer cancel()

	valUpdates := make(chan []*Val, 1)
	go func() {
		for {
			select {
			case currentVals = <-valUpdates:
			case <-abort.Done():
				return
			}

		}
	}()
	go func() {
		Vals(abort, rest, valUpdates)
		cancel()
	}()

	for currentVals == nil || len(currentVals) == 0 {
		time.Sleep(time.Second)
	}

	client, _ := rpchttp.New(rpc, "/websocket")
	err := client.Start()
	if err != nil {
		log.Println(err)
		return
	}
	defer client.Stop()

	status, err := client.Status(abort)
	if err != nil {
		log.Println(err)
		cancel()
	} else {
		ChainID = status.NodeInfo.Network
	}

	currentRound := &NewRound{}
	newRound := make(chan *NewRound, 1)
	bm := bluemonday.StrictPolicy()
	var sameRound int64
	var previousProposer string
	pctUpdate := make(chan float64, 200)
	persistChan := make(chan *redisMsg, 1)

	go func() {
		for {
			select {
			case currentRound = <-newRound:
				if currentRound.Height == sameRound {
					State.Round.Proposer = bm.Sanitize(currentVals[currentRound.Index].Moniker)
					State.Round.ProposerOper = currentVals[currentRound.Index].Valoper
					j, _ := json.Marshal(&NewProposer{
						Type:         "new_proposer",
						Proposer:     bm.Sanitize(currentVals[currentRound.Index].Moniker),
						ProposerOper: currentVals[currentRound.Index].Valoper,
					})
					rounds <- j
					continue
				}
				// double check that we didn't get a new proposer
				if State.Round != nil {
					if m, v := finalProposers.get(State.Round.Height); m != "" && m != State.Round.Proposer {
						log.Printf("proposer changed on block %d, from %s to %s", State.Round.Height, State.Round.Proposer, m)
						State.Round.TimeOutProposer = State.Round.Proposer
						State.Round.Proposer = bm.Sanitize(m)
						State.Round.ProposerOper = v
						j, _ := json.Marshal(&NewProposer{
							Type:            "new_proposer",
							Proposer:        State.Round.Proposer,
							ProposerOper:    State.Round.ProposerOper,
							TimeOutProposer: State.Round.TimeOutProposer,
						})
						rounds <- j
					}
				}
				j, e := json.Marshal(State)
				if e == nil && State.Round != nil && State.Progress != nil && State.PreVotes != nil {
					persistChan <- &redisMsg{
						height: currentRound.Height - 1,
						record: j,
						slow:   false, // TODO: figure out if block was slow!
					}
				}
				previousProposer = currentVals[currentRound.Index].Moniker
				for currentRound.Height != stateHeight {
					time.Sleep(10 * time.Millisecond)
				}
				lastTS = nextTS
				pctUpdate <- 0.0

				sameRound = currentRound.Height
				fmt.Println("round started at:", lastTS, currentRound.Height)
				Percentage = 0
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
		newHeader(abort, client)
		cancel()
	}()
	go func() {
		Round(abort, client, newRound)
		cancel()
	}()
	go func() {
		redisWorker(abort, persistChan)
		cancel()
	}()

	go func() {
		tick := time.NewTicker(50 * time.Millisecond)
		var last, p float64
		for {
			select {
			case p = <-pctUpdate:
			case <-tick.C:
				if last == p || p > 100 {
					continue
				}
				last = p
				State.Progress = &ProgressMsg{
					Type:      "pct",
					Pct:       math.Round(p*100) / 100,
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

	votes := make(chan *VoteState, 1)
	go func() {
		Votes(abort, client, votes, newRound)
		cancel()
	}()
	go func() {
		VoteStream(abort, client, votes)
		cancel()
	}()

	for {
		select {
		case v := <-votes:
			if len(currentVals) == 0 || int32(len(currentVals)) < v.Index || State.Round == nil {
				continue
			}
			newVote := &PreVoteMsg{
				Type:     "prevote",
				Moniker:  currentVals[int(v.Index)].Moniker,
				ValOper:  currentVals[int(v.Index)].Valoper,
				Weight:   float64(math.Floor(100000*currentVals[int(v.Index)].Weight)) / 1000, // three digits of precision, rounded down.
				OffsetMs: v.Time.Sub(lastTS).Milliseconds(),
				Height:   v.Height,
				Proposer: currentVals[int(v.Index)].Moniker == State.Round.Proposer,
			}
			State.PreVotes = append(State.PreVotes, newVote)
			j, e := json.Marshal(newVote)
			if e != nil {
				log.Println(e)
				continue
			}
			Percentage += float64(math.Round(100000*currentVals[int(v.Index)].Weight)) / 1000
			pctUpdate <- Percentage
			updates <- j
		case <-abort.Done():
			return
		}
	}
}
