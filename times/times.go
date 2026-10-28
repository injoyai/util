// Package times 提供保留时区的日历边界计算；时间本身直接使用 time.Time。
package times

import "time"

const (
	FormatDefault = "2006-01-02 15:04:05"
	FormatDate    = "2006-01-02"
	FormatTime    = "15:04:05"
)

func IntegerSecond(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, t.Location())
}
func IntegerMinute(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, t.Location())
}
func IntegerHour(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
}
func IntegerDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// IntegerWeek 返回本周周一的当地零点。
func IntegerWeek(t time.Time) time.Time { return IntegerDay(t).AddDate(0, 0, -(int(t.Weekday())+6)%7) }
func IntegerMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}
func IntegerQuarter(t time.Time) time.Time {
	return time.Date(t.Year(), time.Month((Quarter(t)-1)*3+1), 1, 0, 0, 0, 0, t.Location())
}
func IntegerYear(t time.Time) time.Time { return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location()) }
func Quarter(t time.Time) int           { return (int(t.Month())-1)/3 + 1 }
