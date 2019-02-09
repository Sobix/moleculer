package registry

import (
	"fmt"
	"sync"

	"github.com/moleculer-go/moleculer"
	"github.com/moleculer-go/moleculer/service"
	"github.com/moleculer-go/moleculer/strategy"
	log "github.com/sirupsen/logrus"
)

type EventEntry struct {
	targetNodeID string
	event        *service.Event
	isLocal      bool
}

func (eventEntry EventEntry) TargetNodeID() string {
	return eventEntry.targetNodeID
}

func (eventEntry *EventEntry) IsLocal() bool {
	return eventEntry.isLocal
}

func (eventEntry *EventEntry) String() string {
	return fmt.Sprint("EventEntry Node -> ", eventEntry.targetNodeID,
		" - Service: ", eventEntry.event.ServiceName(),
		" - Event Name: ", eventEntry.event.Name(),
		" - Group: ", eventEntry.event.Group())
}

func catchEventError(context moleculer.BrokerContext, logger *log.Entry) {
	if err := recover(); err != nil {
		logger.Error("local event failed :( event: ", context.EventName(), " error: ", err)
	}
}

func (eventEntry *EventEntry) emitLocalEvent(context moleculer.BrokerContext) {
	logger := context.Logger().WithField("eventCatalog", "emitLocalEvent")
	logger.Debug("Before invoking local event: ", context.EventName())
	defer catchEventError(context, logger)
	handler := eventEntry.event.Handler()
	handler(context.(moleculer.Context), context.Payload())
	logger.Debug("After invoking local event: ", context.EventName())

}

type EventCatalog struct {
	events map[string][]EventEntry
	mutex  *sync.Mutex
	logger *log.Entry
}

func CreateEventCatalog(logger *log.Entry) *EventCatalog {
	events := make(map[string][]EventEntry)
	mutex := &sync.Mutex{}
	return &EventCatalog{events: events, mutex: mutex, logger: logger}
}

// Add a new event to the catalog.
func (eventCatalog *EventCatalog) Add(nodeID string, event service.Event, local bool) {
	entry := EventEntry{nodeID, &event, local}
	name := event.Name()
	eventCatalog.logger.Debug("Add() name: ", name, " serviceName: ", event.ServiceName())
	eventCatalog.events[name] = append(eventCatalog.events[name], entry)
}

func (eventCatalog *EventCatalog) Update(nodeID string, name string, updates map[string]interface{}) {
	//TODO .. the only thing that can be udpated is the Event Schema (validation) and that does not exist yet
}

func (eventCatalog *EventCatalog) Remove(nodeID string, name string) {
	var newList []EventEntry
	options := eventCatalog.events[name]
	for _, event := range options {
		if event.targetNodeID != nodeID {
			newList = append(newList, event)
		}
	}
	eventCatalog.events[name] = newList
}

func matchGroup(event *service.Event, groups []string) bool {
	if groups == nil || len(groups) == 0 {
		return true
	}
	for _, group := range groups {
		if event.Group() == group {
			return true
		}
	}
	return false
}

func findLocal(events []EventEntry) *EventEntry {
	for _, item := range events {
		if item.IsLocal() {
			return &item
		}
	}
	return nil
}

// Next find all events registered in this node and use the strategy to select and return the best one to be called.
func (eventCatalog *EventCatalog) Next(name string, stg strategy.Strategy, groups []string, localOnly bool) []*EventEntry {
	events := eventCatalog.events[name]
	entryGroups := make(map[string][]EventEntry)
	for _, entry := range events {
		if localOnly && !entry.isLocal {
			continue
		}
		if matchGroup(entry.event, groups) {
			entryGroups[entry.event.Group()] = append(entryGroups[entry.event.Group()], entry)
		}
	}
	var result []*EventEntry
	for _, entries := range entryGroups {
		if local := findLocal(entries); local != nil {
			result = append(result, local)
		} else if len(entries) == 1 {
			result = append(result, &entries[0])
		} else if len(entries) > 1 {
			nodes := make([]strategy.Selector, len(entries))
			for index, entry := range entries {
				nodes[index] = &entry
			}
			if selected := stg.Select(nodes); selected != nil {
				entry := (*selected).(EventEntry)
				result = append(result, &entry)
			}
		}
	}
	// for _, item := range result {
	// 	fmt.Println("\n *** eventCatalog.Next() item: ", (*item).event.Name(), " service: ", (*item).event.ServiceName())
	// }

	return result
}
