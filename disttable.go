package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func DistributeTable(db *sql.DB, coll *mongo.Collection) (err error) {
	schema := coll.Database().Name()
	collName := coll.Name()

	// need to insert something to the collection to get it to exist
	doc := struct {
		ID primitive.ObjectID `bson:"_id"`
	}{
		ID: primitive.NewObjectID(),
	}
	_, err = coll.InsertOne(context.TODO(), doc)
	if err != nil {
		return err
	}

	// clear out the record created above since the collection now should exist
	filter := bson.M{"_id": doc.ID}
	_, err = coll.DeleteOne(context.TODO(), filter)
	if err != nil {
		log.Printf("%v; fail to delete record created for distribution; id: %v", err, doc.ID)
	}

	// get table and index names
	var table, index string
	qTmplSelectTableIndex := `
		SELECT _jsonb->>'table'
		, _jsonb->'indexes'->0->>'pgindex'
		FROM %s._ferretdb_database_metadata
		WHERE _jsonb->>'_id' = $1
	`
	err = db.QueryRow(fmt.Sprintf(qTmplSelectTableIndex, schema), collName).Scan(&table, &index)
	if err != nil {
		return err
	}
	log.Println("got table and index name", "table", table, "index", index)

	qTmplDropIndex := `DROP INDEX %s.%s`
	_, err = db.Exec(fmt.Sprintf(qTmplDropIndex, schema, index))
	if err != nil {
		return err
	}
	log.Println("old index dropped")

	qTmplAlterTableName := `ALTER TABLE %s.%s RENAME TO %s_back`
	_, err = db.Exec(fmt.Sprintf(qTmplAlterTableName, schema, table, table))
	if err != nil {
		return err
	}
	log.Println("table rename to ..._back")

	qTmplAddIDColumn := `ALTER TABLE %s.%s_back ADD COLUMN _id TEXT`
	_, err = db.Exec(fmt.Sprintf(qTmplAddIDColumn, schema, table))
	if err != nil {
		return err
	}
	log.Println("add _id field")

	qTmplAddNewPK := `ALTER TABLE %s.%s_back  ADD PRIMARY KEY (_id)`
	_, err = db.Exec(fmt.Sprintf(qTmplAddNewPK, schema, table))
	if err != nil {
		return err
	}
	log.Println("add new PK to match with dist key")

	qDistributeBackTable := `SELECT create_distributed_table($1, '_id')`
	_, err = db.Exec(qDistributeBackTable, schema+"."+table+"_back")
	if err != nil {
		return err
	}
	log.Println("make ..._back table a distributed table")

	qTmplCreateBasicView := `CREATE OR REPLACE VIEW %s.%s AS (SELECT * FROM %s.%s_back)`
	_, err = db.Exec(fmt.Sprintf(qTmplCreateBasicView, schema, table, schema, table))
	if err != nil {
		return err
	}
	log.Println("create view with orignal table name")

	qTmplCreateTriggerFunction := `
		CREATE OR REPLACE FUNCTION %s.func_tg_insert__%s() RETURNS TRIGGER AS $view_insert_row$
		BEGIN
			INSERT INTO %s.%s_back (_id, _jsonb)
			SELECT NEW._jsonb->>'_id', NEW._jsonb;

			RETURN NULL;
		END;
		$view_insert_row$ LANGUAGE plpgsql;
	`
	_, err = db.Exec(fmt.Sprintf(qTmplCreateTriggerFunction, schema, table, schema, table))
	if err != nil {
		return err
	}

	qTmplCreateTrigger := `
		CREATE OR REPLACE TRIGGER tg_insert__%s
		INSTEAD OF INSERT ON %s.%s
		FOR EACH ROW EXECUTE FUNCTION %s.func_tg_insert__%s();
	`
	_, err = db.Exec(fmt.Sprintf(qTmplCreateTrigger, table, schema, table, schema, table))
	if err != nil {
		return err
	}
	log.Println("create trigger to handle inserts")

	return
}

func DropDistributeTable(db *sql.DB, coll *mongo.Collection) (err error) {
	schema := coll.Database().Name()
	collName := coll.Name()

	var table string
	qTmplSelectTableIndex := `
		SELECT _jsonb->>'table'
		FROM %s._ferretdb_database_metadata
		WHERE _jsonb->>'_id' = $1
	`
	err = db.QueryRow(fmt.Sprintf(qTmplSelectTableIndex, schema), collName).Scan(&table)
	if err != nil {
		return err
	}
	log.Println("DropDistributeTable - got table name", "table", table)

	qTmplDropTableCascade := `DROP TABLE %s.%s_back CASCADE`
	_, err = db.Exec(fmt.Sprintf(qTmplDropTableCascade, schema, table))
	if err != nil {
		return err
	}
	log.Println("DropDistributeTable - drop table ..._back cascade")

	return
}
