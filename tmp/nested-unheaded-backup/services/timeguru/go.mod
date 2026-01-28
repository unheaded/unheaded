module github.com/unheaded/unheaded/services/timeguru

go 1.21

require (
	github.com/unheaded/unheaded/pkg/busboy-client v0.0.0
	modernc.org/sqlite v1.28.0
)

replace github.com/unheaded/unheaded/pkg/busboy-client => ../../pkg/busboy-client
