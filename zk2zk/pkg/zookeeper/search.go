package zookeeper

import (
	"fmt"
	"github.com/go-zookeeper/zk"
	"sync"
)

type traverseHandlerWithError func(path string, data []byte, children []string, stat *zk.Stat, err error)

func QuickBfsTraverse(pool *ZkConnGroup, path string, handle traverseHandlerWithError) {
	var wg sync.WaitGroup
	wg.Add(1)
	quickBfsHandle(pool, path, handle, &wg)
	wg.Wait()
}

func quickBfsHandle(pool *ZkConnGroup, path string, handle traverseHandlerWithError, parentWg *sync.WaitGroup) {
	defer parentWg.Done()

	connection := pool.NextCon()
	nodeData, nodeStat, err := connection.Get(path)
	if err != nil {
		handle(path, nil, nil, nil, fmt.Errorf("get path %s error: %v", path, err))
		return
	}

	if nodeStat.EphemeralOwner == 0 && nodeStat.NumChildren > 0 {
		nodeChildren, _, err := connection.Children(path)
		if err != nil {
			if err == zk.ErrNoNode {
				return
			}
			handle(path, nodeData, nil, nodeStat, fmt.Errorf("get path %s nodeChildren error: %v", path, err))
		}

		handle(path, nodeData, nodeChildren, nodeStat, nil)

		if len(nodeChildren) > 0 {
			var wg sync.WaitGroup
			wg.Add(len(nodeChildren))

			for _, v := range nodeChildren {
				var p string
				if path != "/" {
					p = path + "/" + v
				} else {
					p = "/" + v
				}

				go quickBfsHandle(pool, p, handle, &wg)
			}

			wg.Wait()
		}
	} else {
		handle(path, nodeData, nil, nodeStat, nil)
	}
}
