package governance

import (
	d "cognida/internal/model/governance"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Five-field cron supports lists, ranges and steps; schedules are evaluated in
// their stored timezone and return UTC instants, including DST transitions.
type cronField struct {
	values map[int]bool
	wild   bool
}

func cronPart(raw string, min, max int) (cronField, error) {
	f := cronField{values: map[int]bool{}, wild: strings.Contains(raw, "*")}
	for _, part := range strings.Split(raw, ",") {
		step := 1
		base := part
		if pieces := strings.Split(part, "/"); len(pieces) == 2 {
			base = pieces[0]
			n, e := strconv.Atoi(pieces[1])
			if e != nil || n < 1 || n > max-min+1 {
				return f, d.ErrInvalid
			}
			step = n
		} else if len(pieces) > 2 {
			return f, d.ErrInvalid
		}
		lo, hi := min, max
		if base != "*" {
			bounds := strings.Split(base, "-")
			var err error
			lo, err = strconv.Atoi(bounds[0])
			if err != nil {
				return f, d.ErrInvalid
			}
			hi = lo
			if strings.Contains(part, "/") {
				hi = max
			}
			if len(bounds) == 2 {
				hi, err = strconv.Atoi(bounds[1])
				if err != nil {
					return f, d.ErrInvalid
				}
			} else if len(bounds) > 2 {
				return f, d.ErrInvalid
			}
		}
		if lo < min || hi > max || lo > hi {
			return f, d.ErrInvalid
		}
		for n := lo; n <= hi; n += step {
			f.values[n] = true
		}
	}
	return f, nil
}
func NextRun(schedule d.Schedule, after time.Time) (time.Time, error) {
	if len(schedule.Cron) > 128 || schedule.Timezone == "" || len(schedule.Timezone) > 128 {
		return time.Time{}, d.ErrInvalid
	}
	loc, e := time.LoadLocation(schedule.Timezone)
	if e != nil {
		return time.Time{}, fmt.Errorf("%w: timezone", d.ErrInvalid)
	}
	parts := strings.Fields(schedule.Cron)
	if len(parts) != 5 {
		return time.Time{}, fmt.Errorf("%w: cron requires five fields", d.ErrInvalid)
	}
	fields := make([]cronField, 5)
	mins := []int{0, 0, 1, 1, 0}
	maxs := []int{59, 23, 31, 12, 7}
	for i, p := range parts {
		fields[i], e = cronPart(p, mins[i], maxs[i])
		if e != nil {
			return time.Time{}, fmt.Errorf("%w: cron field %d", d.ErrInvalid, i+1)
		}
	}
	next := after.UTC().Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 8*366*24*60; i++ {
		local := next.In(loc)
		day := fields[2].values[local.Day()]
		week := fields[4].values[int(local.Weekday())] || (local.Weekday() == time.Sunday && fields[4].values[7])
		matchesDay := day && week
		if !fields[2].wild && !fields[4].wild {
			matchesDay = day || week
		}
		if fields[0].values[local.Minute()] && fields[1].values[local.Hour()] && fields[3].values[int(local.Month())] && matchesDay {
			return next, nil
		}
		next = next.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("%w: cron has no occurrence within eight years", d.ErrInvalid)
}
