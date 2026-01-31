package template

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// RegisterBuiltinFunctions registers additional built-in functions.
func RegisterBuiltinFunctions(e *Expander) {
	// String functions
	e.RegisterFunc("concat", funcConcat)
	e.RegisterFunc("contains", funcContains)
	e.RegisterFunc("hasPrefix", funcHasPrefix)
	e.RegisterFunc("hasSuffix", funcHasSuffix)
	e.RegisterFunc("repeat", funcRepeat)
	e.RegisterFunc("reverse", funcReverse)
	e.RegisterFunc("title", funcTitle)
	e.RegisterFunc("trimPrefix", funcTrimPrefix)
	e.RegisterFunc("trimSuffix", funcTrimSuffix)
	e.RegisterFunc("padLeft", funcPadLeft)
	e.RegisterFunc("padRight", funcPadRight)

	// Math functions
	e.RegisterFunc("add", funcAdd)
	e.RegisterFunc("sub", funcSub)
	e.RegisterFunc("mul", funcMul)
	e.RegisterFunc("div", funcDiv)
	e.RegisterFunc("mod", funcMod)
	e.RegisterFunc("max", funcMax)
	e.RegisterFunc("min", funcMin)

	// Date/time functions
	e.RegisterFunc("now", funcNow)
	e.RegisterFunc("date", funcDate)
	e.RegisterFunc("formatDate", funcFormatDate)

	// Path functions
	e.RegisterFunc("basename", funcBasename)
	e.RegisterFunc("dirname", funcDirname)
	e.RegisterFunc("ext", funcExt)
	e.RegisterFunc("clean", funcClean)
	e.RegisterFunc("abs", funcAbs)

	// Encoding functions
	e.RegisterFunc("sha256", funcSHA256)
	e.RegisterFunc("hex", funcHex)

	// Conditional functions
	e.RegisterFunc("if", funcIf)
	e.RegisterFunc("eq", funcEq)
	e.RegisterFunc("ne", funcNe)
	e.RegisterFunc("lt", funcLt)
	e.RegisterFunc("gt", funcGt)
	e.RegisterFunc("le", funcLe)
	e.RegisterFunc("ge", funcGe)
	e.RegisterFunc("and", funcAnd)
	e.RegisterFunc("or", funcOr)
	e.RegisterFunc("not", funcNot)

	// System functions
	e.RegisterFunc("hostname", funcHostname)
	e.RegisterFunc("pwd", funcPwd)
	e.RegisterFunc("user", funcUser)
	e.RegisterFunc("home", funcHome)

	// Utility functions
	e.RegisterFunc("coalesce", funcCoalesce)
	e.RegisterFunc("ternary", funcTernary)
	e.RegisterFunc("empty", funcEmpty)
	e.RegisterFunc("len", funcLen)
}

// String functions

func funcConcat(args ...string) (string, error) {
	return strings.Join(args, ""), nil
}

func funcContains(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("contains: requires string and substring")
	}
	if strings.Contains(args[0], args[1]) {
		return "true", nil
	}
	return "false", nil
}

func funcHasPrefix(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("hasPrefix: requires string and prefix")
	}
	if strings.HasPrefix(args[0], args[1]) {
		return "true", nil
	}
	return "false", nil
}

func funcHasSuffix(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("hasSuffix: requires string and suffix")
	}
	if strings.HasSuffix(args[0], args[1]) {
		return "true", nil
	}
	return "false", nil
}

func funcRepeat(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("repeat: requires string and count")
	}
	count, err := strconv.Atoi(args[1])
	if err != nil {
		return "", fmt.Errorf("repeat: invalid count: %s", args[1])
	}
	return strings.Repeat(args[0], count), nil
}

func funcReverse(args ...string) (string, error) {
	if len(args) < 1 {
		return "", nil
	}
	runes := []rune(args[0])
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes), nil
}

func funcTitle(args ...string) (string, error) {
	if len(args) < 1 {
		return "", nil
	}
	return strings.Title(args[0]), nil
}

func funcTrimPrefix(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("trimPrefix: requires string and prefix")
	}
	return strings.TrimPrefix(args[0], args[1]), nil
}

func funcTrimSuffix(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("trimSuffix: requires string and suffix")
	}
	return strings.TrimSuffix(args[0], args[1]), nil
}

func funcPadLeft(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("padLeft: requires string and width")
	}
	width, err := strconv.Atoi(args[1])
	if err != nil {
		return "", fmt.Errorf("padLeft: invalid width: %s", args[1])
	}
	padChar := " "
	if len(args) > 2 {
		padChar = args[2]
	}
	s := args[0]
	for len(s) < width {
		s = padChar + s
	}
	return s, nil
}

func funcPadRight(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("padRight: requires string and width")
	}
	width, err := strconv.Atoi(args[1])
	if err != nil {
		return "", fmt.Errorf("padRight: invalid width: %s", args[1])
	}
	padChar := " "
	if len(args) > 2 {
		padChar = args[2]
	}
	s := args[0]
	for len(s) < width {
		s = s + padChar
	}
	return s, nil
}

// Math functions

func funcAdd(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("add: requires two numbers")
	}
	a, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		return "", fmt.Errorf("add: invalid number: %s", args[0])
	}
	b, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return "", fmt.Errorf("add: invalid number: %s", args[1])
	}
	result := a + b
	if result == float64(int64(result)) {
		return strconv.FormatInt(int64(result), 10), nil
	}
	return strconv.FormatFloat(result, 'f', -1, 64), nil
}

func funcSub(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("sub: requires two numbers")
	}
	a, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		return "", fmt.Errorf("sub: invalid number: %s", args[0])
	}
	b, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return "", fmt.Errorf("sub: invalid number: %s", args[1])
	}
	result := a - b
	if result == float64(int64(result)) {
		return strconv.FormatInt(int64(result), 10), nil
	}
	return strconv.FormatFloat(result, 'f', -1, 64), nil
}

func funcMul(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("mul: requires two numbers")
	}
	a, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		return "", fmt.Errorf("mul: invalid number: %s", args[0])
	}
	b, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return "", fmt.Errorf("mul: invalid number: %s", args[1])
	}
	result := a * b
	if result == float64(int64(result)) {
		return strconv.FormatInt(int64(result), 10), nil
	}
	return strconv.FormatFloat(result, 'f', -1, 64), nil
}

func funcDiv(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("div: requires two numbers")
	}
	a, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		return "", fmt.Errorf("div: invalid number: %s", args[0])
	}
	b, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return "", fmt.Errorf("div: invalid number: %s", args[1])
	}
	if b == 0 {
		return "", fmt.Errorf("div: division by zero")
	}
	result := a / b
	if result == float64(int64(result)) {
		return strconv.FormatInt(int64(result), 10), nil
	}
	return strconv.FormatFloat(result, 'f', -1, 64), nil
}

func funcMod(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("mod: requires two integers")
	}
	a, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return "", fmt.Errorf("mod: invalid integer: %s", args[0])
	}
	b, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return "", fmt.Errorf("mod: invalid integer: %s", args[1])
	}
	if b == 0 {
		return "", fmt.Errorf("mod: division by zero")
	}
	return strconv.FormatInt(a%b, 10), nil
}

func funcMax(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("max: requires at least two numbers")
	}
	max, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		return "", fmt.Errorf("max: invalid number: %s", args[0])
	}
	for _, arg := range args[1:] {
		val, err := strconv.ParseFloat(arg, 64)
		if err != nil {
			return "", fmt.Errorf("max: invalid number: %s", arg)
		}
		if val > max {
			max = val
		}
	}
	if max == float64(int64(max)) {
		return strconv.FormatInt(int64(max), 10), nil
	}
	return strconv.FormatFloat(max, 'f', -1, 64), nil
}

func funcMin(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("min: requires at least two numbers")
	}
	min, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		return "", fmt.Errorf("min: invalid number: %s", args[0])
	}
	for _, arg := range args[1:] {
		val, err := strconv.ParseFloat(arg, 64)
		if err != nil {
			return "", fmt.Errorf("min: invalid number: %s", arg)
		}
		if val < min {
			min = val
		}
	}
	if min == float64(int64(min)) {
		return strconv.FormatInt(int64(min), 10), nil
	}
	return strconv.FormatFloat(min, 'f', -1, 64), nil
}

// Date/time functions

func funcNow(args ...string) (string, error) {
	format := time.RFC3339
	if len(args) > 0 {
		format = args[0]
	}
	return time.Now().Format(format), nil
}

func funcDate(args ...string) (string, error) {
	return time.Now().Format("2006-01-02"), nil
}

func funcFormatDate(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("formatDate: requires timestamp and format")
	}
	ts, err := time.Parse(time.RFC3339, args[0])
	if err != nil {
		// Try Unix timestamp
		unix, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return "", fmt.Errorf("formatDate: invalid timestamp: %s", args[0])
		}
		ts = time.Unix(unix, 0)
	}
	return ts.Format(args[1]), nil
}

// Path functions

func funcBasename(args ...string) (string, error) {
	if len(args) < 1 {
		return "", nil
	}
	return filepath.Base(args[0]), nil
}

func funcDirname(args ...string) (string, error) {
	if len(args) < 1 {
		return "", nil
	}
	return filepath.Dir(args[0]), nil
}

func funcExt(args ...string) (string, error) {
	if len(args) < 1 {
		return "", nil
	}
	return filepath.Ext(args[0]), nil
}

func funcClean(args ...string) (string, error) {
	if len(args) < 1 {
		return "", nil
	}
	return filepath.Clean(args[0]), nil
}

func funcAbs(args ...string) (string, error) {
	if len(args) < 1 {
		return "", nil
	}
	return filepath.Abs(args[0])
}

// Encoding functions

func funcSHA256(args ...string) (string, error) {
	if len(args) < 1 {
		return "", nil
	}
	hash := sha256.Sum256([]byte(args[0]))
	return fmt.Sprintf("%x", hash), nil
}

func funcHex(args ...string) (string, error) {
	if len(args) < 1 {
		return "", nil
	}
	return fmt.Sprintf("%x", args[0]), nil
}

// Conditional functions

func funcIf(args ...string) (string, error) {
	if len(args) < 3 {
		return "", fmt.Errorf("if: requires condition, true value, and false value")
	}
	condition := args[0]
	trueVal := args[1]
	falseVal := args[2]

	if condition == "true" || condition == "1" || condition != "" && condition != "false" && condition != "0" {
		return trueVal, nil
	}
	return falseVal, nil
}

func funcEq(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("eq: requires two values")
	}
	if args[0] == args[1] {
		return "true", nil
	}
	return "false", nil
}

func funcNe(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("ne: requires two values")
	}
	if args[0] != args[1] {
		return "true", nil
	}
	return "false", nil
}

func funcLt(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("lt: requires two numbers")
	}
	a, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		return "", fmt.Errorf("lt: invalid number: %s", args[0])
	}
	b, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return "", fmt.Errorf("lt: invalid number: %s", args[1])
	}
	if a < b {
		return "true", nil
	}
	return "false", nil
}

func funcGt(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("gt: requires two numbers")
	}
	a, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		return "", fmt.Errorf("gt: invalid number: %s", args[0])
	}
	b, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return "", fmt.Errorf("gt: invalid number: %s", args[1])
	}
	if a > b {
		return "true", nil
	}
	return "false", nil
}

func funcLe(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("le: requires two numbers")
	}
	a, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		return "", fmt.Errorf("le: invalid number: %s", args[0])
	}
	b, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return "", fmt.Errorf("le: invalid number: %s", args[1])
	}
	if a <= b {
		return "true", nil
	}
	return "false", nil
}

func funcGe(args ...string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("ge: requires two numbers")
	}
	a, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		return "", fmt.Errorf("ge: invalid number: %s", args[0])
	}
	b, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return "", fmt.Errorf("ge: invalid number: %s", args[1])
	}
	if a >= b {
		return "true", nil
	}
	return "false", nil
}

func funcAnd(args ...string) (string, error) {
	for _, arg := range args {
		if arg == "" || arg == "false" || arg == "0" {
			return "false", nil
		}
	}
	return "true", nil
}

func funcOr(args ...string) (string, error) {
	for _, arg := range args {
		if arg != "" && arg != "false" && arg != "0" {
			return "true", nil
		}
	}
	return "false", nil
}

func funcNot(args ...string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("not: requires a value")
	}
	if args[0] == "" || args[0] == "false" || args[0] == "0" {
		return "true", nil
	}
	return "false", nil
}

// System functions

func funcHostname(args ...string) (string, error) {
	return os.Hostname()
}

func funcPwd(args ...string) (string, error) {
	return os.Getwd()
}

func funcUser(args ...string) (string, error) {
	return os.Getenv("USER"), nil
}

func funcHome(args ...string) (string, error) {
	return os.UserHomeDir()
}

// Utility functions

func funcCoalesce(args ...string) (string, error) {
	for _, arg := range args {
		if arg != "" {
			return arg, nil
		}
	}
	return "", nil
}

func funcTernary(args ...string) (string, error) {
	return funcIf(args...)
}

func funcEmpty(args ...string) (string, error) {
	if len(args) < 1 || args[0] == "" {
		return "true", nil
	}
	return "false", nil
}

func funcLen(args ...string) (string, error) {
	if len(args) < 1 {
		return "0", nil
	}
	return strconv.Itoa(len(args[0])), nil
}
