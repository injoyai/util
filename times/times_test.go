package times

import (
	"testing"
	"time"
	_ "time/tzdata"
)

func TestCalendarBoundaries(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	date := time.Date(2024, 3, 10, 15, 23, 45, 999, loc) // 周日，夏令时开始当天。
	week := IntegerWeek(date)
	if week.Weekday() != time.Monday || week.Day() != 4 || week.Hour() != 0 || week.Location() != loc {
		t.Fatal(week)
	}
	if date.Sub(IntegerDay(date)) != 14*time.Hour+23*time.Minute+45*time.Second+999 {
		t.Fatal("DST boundary")
	}
	for month := time.January; month <= time.December; month++ {
		d := time.Date(2024, month, 29, 15, 23, 45, 999, loc)
		if q := Quarter(d); q != (int(month)-1)/3+1 || IntegerQuarter(d).Month() != time.Month((q-1)*3+1) {
			t.Fatal(month)
		}
	}
	if IntegerSecond(date).Nanosecond() != 0 || IntegerMinute(date).Second() != 0 || IntegerHour(date).Minute() != 0 || IntegerMonth(date).Day() != 1 || IntegerYear(date).Month() != 1 {
		t.Fatal("boundary")
	}
}
