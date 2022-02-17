package pvm

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	"log"
	"strconv"
	"sync"
	"time"
)

type redisMsg struct {
	height int64
	record []byte
	slow   bool
}

const maxRecords = 500_000 // 500k is probably around 16MiB of RAM.
func BlockNotFound(height string) []byte {
	h, _ := strconv.Atoi(height)
	return []byte(fmt.Sprintf(`{"round":{"height":"%d","proposer":"Block Not Found","type":"round"},"progress":{"type":"pct","pct":0},"pre_votes":[]}`, h))
}

func redisWorker(ctx context.Context, save chan *redisMsg) {

	// background worker to cleanup old redis records:
	go func() {
		tick := time.NewTicker(10 * time.Minute)
		for {
			select {
			case <-tick.C:
				if Cache.Highest == 0 {
					continue
				}
				log.Println("cleaning old records from db")
				rdb, err := getRedisClient()
				if err != nil {
					log.Println("could not clean historic redis records", err)
					continue
				}
				timeout, cancel := context.WithTimeout(context.Background(), time.Minute)
				keys, err := rdb.Keys(timeout, "*").Result()
				cancel()
				if err != nil {
					log.Println("could not clean historic redis records", err)
					continue
				}
				var highest int
				index := make([]int, 0)
				for _, k := range keys {
					i, e := strconv.Atoi(k)
					if e != nil {
						continue
					}
					index = append(index, i)
					if i < highest {
						highest = i
					}
					keys = make([]string, 0)
					for _, key := range index {
						if key < highest-maxRecords {
							s := strconv.Itoa(key)
							if s != "" {
								keys = append(keys, s)
							}
						}
					}
					if len(keys) > 0 {
						if failed := rdb.Del(ctx, keys...).Err(); failed != nil {
							log.Println("could not delete old keys", failed)
						}
					}
					log.Printf("done cleaning records, removed %d keys", len(keys))
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case m := <-save:
			e := saveRecord(m.height, m.record, m.slow)
			if e != nil {
				log.Println("could not save record to redis", e)
			}
		case <-ctx.Done():
			return
		}
	}
}

func getRedisClient() (rdb *redis.Client, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rdb = &redis.Client{}
	if redisTls {
		rdb = redis.NewClient(&redis.Options{
			Addr:     redisUrl,
			Password: redisPass,
			DB:       redisDb,
			TLSConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		})
	} else {
		rdb = redis.NewClient(&redis.Options{
			Addr:     redisUrl,
			Password: redisPass,
			DB:       redisDb,
		})
	}
	err = rdb.Ping(ctx).Err()
	return
}

func saveRecord(height int64, record []byte, slow bool) error {
	if record == nil || len(record) == 0 || height == 0 {
		return errors.New("invalid record")
	}
	rdb, err := getRedisClient()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if slow {
		err = rdb.Set(ctx, fmt.Sprintf("slow-%d", height), record, 0).Err()
	}
	return rdb.Set(ctx, fmt.Sprintf("%d", height), record, 0).Err()
}

func FetchRecord(height int64) ([]byte, error) {
	if height == 0 {
		return BlockNotFound("0"), errors.New("invalid height")
	}
	if ok, record := Cache.get(height); ok {
		return record, nil
	}
	rdb, err := getRedisClient()
	if err != nil {
		return BlockNotFound(fmt.Sprintf("%d", height)), err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	record, err := rdb.Get(ctx, fmt.Sprintf("%d", height)).Result()
	return []byte(record), err
}

type LocalCache struct {
	sync.RWMutex
	States  map[int64][]byte
	Highest int64
}

func newLocalCache() *LocalCache {
	return &LocalCache{
		RWMutex: sync.RWMutex{},
		States:  make(map[int64][]byte),
		Highest: 0,
	}
}

func (lc *LocalCache) trim() {
	lc.Lock()
	defer lc.Unlock()
	for k := range lc.States {
		if k < lc.Highest-10 {
			delete(lc.States, k)
		}
	}
}

func (lc *LocalCache) get(height int64) (ok bool, record []byte) {
	lc.RLock()
	defer lc.RUnlock()
	record = lc.States[height]
	if record != nil {
		ok = true
	}
	return
}

func (lc *LocalCache) add(height int64, record []byte) {
	if record != nil && height != 0 {
		lc.Lock()
		lc.States[height] = record
		lc.Highest = height
		lc.Unlock()
		go lc.trim()
	}
}
