package main

import (
	"context"
	"database/sql"
	"fmt"
	student "geevly/events/gen/proto/go"

	"github.com/Howard3/gosignal"
	"github.com/Howard3/gosignal/drivers/eventstore"
	"github.com/Howard3/gosignal/drivers/queue"
	src "github.com/Howard3/gosignal/sourcing"
	_ "github.com/mattn/go-sqlite3"
)

func createAggTable(db *sql.DB) {
	// TODO: move to migration system
	createTable := `CREATE TABLE IF NOT EXISTS sampleagg_events (
		id SERIAL PRIMARY KEY,
		type VARCHAR(255) NOT NULL,
		data BYTEA NOT NULL,
		version INT NOT NULL,
		timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
		aggregate_id VARCHAR(255) NOT NULL,
		UNIQUE (aggregate_id, version)
	 );`

	_, err := db.Exec(createTable)
	if err != nil {
		panic(fmt.Sprintf("Error creating table: %s", err))
	}
}

func main() {
	mq := queue.MemoryQueue{}

	go subscribeSampleEvent(context.Background(), &mq)

	// configure a sqlite connection
	db, err := sql.Open("sqlite3", "test.db")
	if err != nil {
		panic(fmt.Sprintf("Error opening database: %s", err))
	}

	// create the table
	createAggTable(db)

	// configure event store
	es := eventstore.SQLStore{DB: db, TableName: "sampleagg_events"}

	// configure repo
	repo := src.NewRepository(src.WithEventStore(es), src.WithQueue(&mq))

	ctx := context.Background()
	agg := StudentAggregate{}

	evt, err := agg.CreateStudent(&student.AddStudentEvent{
		FirstName: "John",
		LastName:  "Doe",
	})

	if err != nil {
		// TODO: handle error
		panic(fmt.Sprintf("Error creating student: %s", err))
	}

	evt2, err := agg.SetStudentStatus(&student.SetStudentStatusEvent{
		Status: student.StudentStatus_ACTIVE,
	})

	if err != nil {
		panic(fmt.Sprintf("Error setting student status: %s", err))
	}

	// add event
	err = repo.Store(ctx, []gosignal.Event{*evt, *evt2})
	if err != nil {
		panic(fmt.Sprintf("Error storing aggregate: %s", err))
	}

	if err := repo.Load(ctx, agg.GetID(), &agg, nil); err != nil {
		panic(fmt.Sprintf("Error loading aggregate: %s", err))
	}
}

func subscribeSampleEvent(ctx context.Context, mq *queue.MemoryQueue) {
	_, ch, err := mq.Subscribe(EVENT_ADD_STUDENT)
	if err != nil {
		panic(fmt.Sprintf("Error subscribing to queue: %s", err))
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			fmt.Println("received event data: " + string(msg.Message()))
		}
	}
}
