package pvm

import (
	"context"
	"github.com/tendermint/tendermint/libs/json"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"
	"sort"
	"time"
)

type Val struct {
	Moniker string
	Valoper string
	Weight  float64
	tokens  uint64
	pubkey  string
}

type TrimmedVal struct {
	Validators []struct {
		OperatorAddress string `json:"operator_address"`
		ConsensusPubkey struct {
			Key string `json:"key"`
		} `json:"consensus_pubkey"`
		Jailed      bool   `json:"jailed"`
		Status      string `json:"status"`
		Description struct {
			Moniker string `json:"moniker"`
		} `json:"description"`
		Tokens string `json:"tokens"`
	} `json:"validators"`
}

func Vals(ctx context.Context, rest string, vals chan []*Val) {
	tick := time.NewTicker(6 * time.Second)
	failed, cancel := context.WithCancel(context.Background())
	defer cancel()
	for {
		select {
		case <-failed.Done():
			return
		case <-ctx.Done():
			return
		case <-tick.C:
			func() {
				resp, err := http.Get(rest + "/cosmos/staking/v1beta1/validators?pagination.limit=200")
				if err != nil {
					log.Println(err)
					cancel()
					return
				}
				defer resp.Body.Close()
				b, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					log.Println(err)
					cancel()
					return
				}
				valset := &TrimmedVal{}
				err = json.Unmarshal(b, valset)
				if err != nil {
					log.Println(err)
					cancel()
					return
				}
				updated := make([]*Val, 0)
				totalVotes := new(big.Float)
				for _, v := range valset.Validators {
					if v.Jailed || v.Status != "BOND_STATUS_BONDED" {
						continue
					}
					tokens, ok := new(big.Float).SetString(v.Tokens)
					if !ok {
						log.Println("could not convert to big.Float:", v.Tokens)
						continue
					}
					totalVotes = new(big.Float).Add(tokens, totalVotes)
					tokensInt, _ := tokens.Int64()
					updated = append(updated, &Val{
						Moniker: v.Description.Moniker,
						Valoper: v.OperatorAddress,
						tokens:  uint64(tokensInt),
						pubkey:  v.ConsensusPubkey.Key,
					})
				}
				if len(updated) == 0 {
					log.Println("no validators found!")
					return
				}
				for i := range updated {
					updated[i].Weight, _ = new(big.Float).Quo(new(big.Float).SetUint64(updated[i].tokens), totalVotes).Float64()
				}
				// potentially problematic if two validators have same number of tokens.
				// FIXME: how does TM order in the case of a tie?
				sort.Slice(updated, func(i, j int) bool {
					return updated[i].tokens > updated[j].tokens
				})
				vals <- updated
			}()
		}
	}
}
