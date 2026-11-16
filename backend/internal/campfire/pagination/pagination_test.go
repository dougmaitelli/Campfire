package pagination_test

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"campfire/internal/campfire/pagination"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestParse(t *testing.T) {
	for _, raw := range []string{"page=0", "page=-1", "page=", "page_size=101", "page_size=0", "page_size=1&page_size=2", "page=1&page=2", "page=2147483647&page_size=100", "page=999999999999999999999999999", "before=", "limit=10", "cursor=x"} {
		values, _ := url.ParseQuery(raw)
		if _, err := pagination.Parse(values); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}

	request, err := pagination.Parse(url.Values{})
	if err != nil || request.Page != 1 || request.Size != 50 {
		t.Fatal(request, err)
	}

	request, err = pagination.Parse(url.Values{"page": {"3"}, "page_size": {"20"}})
	if err != nil || request.Page != 3 || request.Size != 20 {
		t.Fatal(request, err)
	}
}

func TestReadFiltersBeforeCountAndPages(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "pages.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	sql, _ := db.DB()

	t.Cleanup(func() { _ = sql.Close() })

	type row struct {
		ID      int
		Visible bool
		At      int
	}

	if err := db.AutoMigrate(&row{}); err != nil {
		t.Fatal(err)
	}

	for id := 1; id <= 107; id++ {
		if err := db.Create(&row{ID: id, Visible: id%2 == 1, At: 1}).Error; err != nil {
			t.Fatal(err)
		}
	}

	seen := map[int]bool{}

	for number := 1; number <= 4; number++ {
		page, err := pagination.Read[row](db.Model(&row{}).Where("visible = ?", true).Order("at DESC, id DESC"), pagination.Request{Page: number, Size: 20})
		if err != nil || page.Pagination.TotalItems != 54 || page.Pagination.TotalPages != 3 || page.Pagination.Page != number {
			t.Fatal(page, err)
		}

		expected := 20
		if number == 3 {
			expected = 14
		}

		if number == 4 {
			expected = 0
		}

		if page.Items == nil || len(page.Items) != expected {
			t.Fatal(page)
		}

		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatal("duplicate", item.ID)
			}

			seen[item.ID] = true
		}
	}

	if len(seen) != 54 {
		t.Fatal("inaccessible rows or missing results", len(seen))
	}
}

func TestReadDoesNotSortCountOrRemovePageOrdering(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "count.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	sql, _ := db.DB()

	t.Cleanup(func() { _ = sql.Close() })

	type row struct {
		ID string `gorm:"primaryKey"`
		At int    `gorm:"index"`
	}

	if err := db.AutoMigrate(&row{}); err != nil {
		t.Fatal(err)
	}

	if err := db.Create(&[]row{{ID: "a", At: 1}, {ID: "c", At: 1}, {ID: "b", At: 1}}).Error; err != nil {
		t.Fatal(err)
	}

	var statements []string

	if err := db.Callback().Query().After("gorm:query").Register("test:pagination-sql", func(tx *gorm.DB) {
		if !tx.DryRun {
			statements = append(statements, tx.Statement.SQL.String())
		}
	}); err != nil {
		t.Fatal(err)
	}

	query := db.Model(&row{}).Order("at DESC, id DESC")

	page, err := pagination.Read[row](query, pagination.Request{Page: 2, Size: 1})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "b" || page.Pagination.TotalItems != 3 {
		t.Fatal(page, err)
	}

	if len(statements) != 2 || strings.Contains(strings.ToUpper(statements[0]), "ORDER BY") || !strings.Contains(statements[1], "ORDER BY at DESC, id DESC") {
		t.Fatal("count must omit ordering while page retains it", statements)
	}

	var plan []struct{ Detail string }
	if err := db.Raw("EXPLAIN QUERY PLAN " + statements[0]).Scan(&plan).Error; err != nil {
		t.Fatal(err)
	}

	for _, step := range plan {
		if strings.Contains(step.Detail, "TEMP B-TREE") {
			t.Fatal("count unexpectedly sorts history", plan)
		}
	}
}
