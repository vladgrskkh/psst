# Quick capture

Pass the whole capture text as one quoted argument:

```sh
psst new "buy milk"
psst new "buy milk @tomorrow 18:30 #groceries"
psst new "buy milk @in 2w at 18:30 #groceries"
psst new "call mom @in 2d at 9pm #family stuff"
psst new "stretch @in 30m"
```

`@` introduces a deadline hint. `#` introduces a list name and must come last;
list names may contain spaces. Omitting the list uses the configured default.

Relative offsets use `m` (minutes), `h` (hours), `d` (calendar days), or `w`
(calendar weeks). Without an explicit time, they retain the time implied by
the offset: `@in 30m` is thirty minutes from now and `@in 2w` is the same local
time two weeks from now.

Calendar day/week offsets may set a clock time with or without `at`:
`@in 2w at 18:30` and `@in 2w 18:30` mean the same thing. The clock uses the
local time zone, including daylight-saving changes; AM/PM clocks such as
`@in 2d at 9pm` and `@in 2d at 9 pm` also work. Hour/minute offsets do
not accept a clock override; use a day/week offset or a concrete date instead.
