package db

import (
	"github.com/anomalous69/fchannel/activitypub"
	"github.com/anomalous69/fchannel/config"
	"github.com/anomalous69/fchannel/util"
)

type Reports struct {
	ID     string
	Count  int
	Actor  activitypub.Actor
	Object activitypub.ObjectBase
	OP     string
	Reason []string
}

type Report struct {
	ID     string
	Reason string
}

type Removed struct {
	ID    string
	Type  string
	Board string
}

func CloseLocalReport(id string, board string) error {
	query := `delete from reported where id=$1 and board=$2`
	_, err := config.DB.Exec(query, id, board)

	return util.MakeError(err, "CloseLocalReportDB")
}

func CreateLocalReport(id string, board string, reason string) error {
	query := `insert into reported (id, count, board, reason) values ($1, $2, $3, $4)`
	_, err := config.DB.Exec(query, id, 1, board, reason)

	return util.MakeError(err, "CreateLocalReportDB")
}

func GetLocalReport(board string) (map[string]Reports, error) {
	var reported = make(map[string]Reports)

	query := `select id, reason from reported where board=$1`
	rows, err := config.DB.Query(query, board)

	if err != nil {
		return reported, util.MakeError(err, "GetLocalReportDB")
	}

	defer rows.Close()
	for rows.Next() {
		var r Report

		if err := rows.Scan(&r.ID, &r.Reason); err != nil {
			return reported, util.MakeError(err, "GetLocalReportDB")
		}

		if report, has := reported[r.ID]; has {
			report.Count += 1
			report.Reason = append(report.Reason, r.Reason)
			reported[r.ID] = report
			continue
		}

		var obj = activitypub.ObjectBase{Id: r.ID}

		col, _ := obj.GetCollectionFromPath()

		if len(col.OrderedItems) == 0 {
			continue
		}

		OP, _ := obj.GetOP()

		reported[r.ID] = Reports{
			ID:     r.ID,
			Count:  1,
			Object: col.OrderedItems[0],
			OP:     OP,
			Actor:  activitypub.Actor{Name: board, Outbox: config.Domain + "/" + board + "/outbox"},
			Reason: []string{r.Reason},
		}
	}

	return reported, nil
}

type ReportsSortDesc []Reports

func (a ReportsSortDesc) Len() int { return len(a) }
func (a ReportsSortDesc) Less(i, j int) bool {
	if a[i].Object.Updated == nil && a[j].Object.Updated == nil {
		return true
	} else if a[i].Object.Updated == nil {
		return false
	} else if a[j].Object.Updated == nil {
		return true
	}
	return a[i].Object.Updated.After(*a[j].Object.Updated)
}
func (a ReportsSortDesc) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
