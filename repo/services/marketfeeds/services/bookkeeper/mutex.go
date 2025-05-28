package bookkeeper

import (
	"bytes"
	"sort"
	"sync"

	"github.com/google/uuid"
)

type usersMutex struct {
	locks sync.Map
}

func (a *usersMutex) userMutex(userID uuid.UUID) *sync.Mutex {
	lock, _ := a.locks.LoadOrStore(userID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func (a *usersMutex) lockUser(userID uuid.UUID) func() {
	mutex := a.userMutex(userID)
	mutex.Lock()
	return mutex.Unlock
}

func (a *usersMutex) lockUsers(userIDs []uuid.UUID) func() {
	sort.Slice(userIDs, func(i, j int) bool {
		return bytes.Compare(userIDs[i][:], userIDs[j][:]) < 0
	})

	mutexes := make([]*sync.Mutex, len(userIDs))
	for i, userID := range userIDs {
		mutex := a.userMutex(userID)
		mutex.Lock()
		mutexes[i] = mutex
	}

	return func() {
		for _, mutex := range mutexes {
			mutex.Unlock()
		}
	}
}
