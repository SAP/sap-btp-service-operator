package template

// genericMap from https://github.com/Masterminds/sprig/blob/master/functions.go
// comment out all unwanted functions
var allowedSprigFunctions = map[string]interface{}{
	"hello": nil,

	// Date functions
	"ago":              nil,
	"date":             nil,
	"date_in_zone":     nil,
	"date_modify":      nil,
	"dateInZone":       nil,
	"dateModify":       nil,
	"duration":         nil,
	"durationRound":    nil,
	"htmlDate":         nil,
	"htmlDateInZone":   nil,
	"must_date_modify": nil,
	"mustDateModify":   nil,
	"mustToDate":       nil,
	"now":              nil,
	"toDate":           nil,
	"unixEpoch":        nil,

	// Strings
	"abbrev":     nil,
	"abbrevboth": nil,
	"trunc":      nil,
	"trim":       nil,
	"upper":      nil,
	"lower":      nil,
	"title":      nil,
	"untitle":    nil,
	"substr":     nil,
	// Switch order so that "foo" | repeat 5
	"repeat": nil,
	// Deprecated: Use trimAll.
	//"trimall": nil,
	// Switch order so that "$foo" | trimall "$"
	"trimAll":      nil,
	"trimSuffix":   nil,
	"trimPrefix":   nil,
	"nospace":      nil,
	"initials":     nil,
	"randAlphaNum": nil,
	"randAlpha":    nil,
	"randAscii":    nil,
	"randNumeric":  nil,
	"swapcase":     nil,
	"shuffle":      nil,
	"snakecase":    nil,
	"camelcase":    nil,
	"kebabcase":    nil,
	"wrap":         nil,
	"wrapWith":     nil,
	// Switch order so that "foobar" | contains "foo"
	"contains":   nil,
	"hasPrefix":  nil,
	"hasSuffix":  nil,
	"quote":      nil,
	"squote":     nil,
	"cat":        nil,
	"indent":     nil,
	"nindent":    nil,
	"replace":    nil,
	"plural":     nil,
	"sha1sum":    nil,
	"sha256sum":  nil,
	"adler32sum": nil,
	"toString":   nil,

	// Wrap Atoi to stop errors.
	"atoi":      nil,
	"int64":     nil,
	"int":       nil,
	"float64":   nil,
	"seq":       nil,
	"toDecimal": nil,

	// split "/" foo/bar returns map[int]string{0: foo, 1: bar}
	"split":     nil,
	"splitList": nil,
	// splitn "/" foo/bar/fuu returns map[int]string{0: foo, 1: bar/fuu}
	"splitn":    nil,
	"toStrings": nil,

	"until":     nil,
	"untilStep": nil,

	// VERY basic arithmetic.
	"add1":    nil,
	"add":     nil,
	"sub":     nil,
	"div":     nil,
	"mod":     nil,
	"mul":     nil,
	"randInt": nil,
	"add1f":   nil,
	"addf":    nil,
	"subf":    nil,
	"divf":    nil,
	"mulf":    nil,
	"biggest": nil,
	"max":     nil,
	"min":     nil,
	"maxf":    nil,
	"minf":    nil,
	"ceil":    nil,
	"floor":   nil,
	"round":   nil,

	// string slices. Note that we reverse the order b/c that's better
	// for template processing.
	"join":      nil,
	"sortAlpha": nil,

	// Defaults
	"default":          nil,
	"empty":            nil,
	"coalesce":         nil,
	"all":              nil,
	"any":              nil,
	"compact":          nil,
	"mustCompact":      nil,
	"fromJson":         nil,
	"toJson":           nil,
	"toPrettyJson":     nil,
	"toRawJson":        nil,
	"mustFromJson":     nil,
	"mustToJson":       nil,
	"mustToPrettyJson": nil,
	"mustToRawJson":    nil,
	"ternary":          nil,
	"deepCopy":         nil,
	"mustDeepCopy":     nil,

	// Reflection
	"typeOf":     nil,
	"typeIs":     nil,
	"typeIsLike": nil,
	"kindOf":     nil,
	"kindIs":     nil,
	"deepEqual":  nil,

	// OS:
	// "env":       nil,
	// "expandenv": nil,

	// Network:
	// "getHostByName": nil,

	// Paths:
	"base":  nil,
	"dir":   nil,
	"clean": nil,
	"ext":   nil,
	"isAbs": nil,

	// Filepaths:
	// "osBase":  nil,
	// "osClean": nil,
	// "osDir":   nil,
	// "osExt":   nil,
	// "osIsAbs": nil,

	// Encoding:
	"b64enc": nil,
	"b64dec": nil,
	"b32enc": nil,
	"b32dec": nil,

	// Data Structures:
	"tuple":              nil, // FIXME: with the addition of append/prepend these are no longer immutable.
	"list":               nil,
	"dict":               nil,
	"get":                nil,
	"set":                nil,
	"unset":              nil,
	"hasKey":             nil,
	"pluck":              nil,
	"keys":               nil,
	"pick":               nil,
	"omit":               nil,
	"merge":              nil,
	"mergeOverwrite":     nil,
	"mustMerge":          nil,
	"mustMergeOverwrite": nil,
	"values":             nil,

	"append": nil, "push": nil,
	"mustAppend": nil, "mustPush": nil,
	"prepend":     nil,
	"mustPrepend": nil,
	"first":       nil,
	"mustFirst":   nil,
	"rest":        nil,
	"mustRest":    nil,
	"last":        nil,
	"mustLast":    nil,
	"initial":     nil,
	"mustInitial": nil,
	"reverse":     nil,
	"mustReverse": nil,
	"uniq":        nil,
	"mustUniq":    nil,
	"without":     nil,
	"mustWithout": nil,
	"has":         nil,
	"mustHas":     nil,
	"slice":       nil,
	"mustSlice":   nil,
	"concat":      nil,
	"dig":         nil,
	"chunk":       nil,
	"mustChunk":   nil,

	// Crypto:
	// "bcrypt":                   nil,
	// "htpasswd":                 nil,
	// "genPrivateKey":            nil,
	// "derivePassword":           nil,
	// "buildCustomCert":          nil,
	// "genCA":                    nil,
	// "genCAWithKey":             nil,
	// "genSelfSignedCert":        nil,
	// "genSelfSignedCertWithKey": nil,
	// "genSignedCert":            nil,
	// "genSignedCertWithKey":     nil,
	// "encryptAES":               nil,
	// "decryptAES":               nil,
	// "randBytes":                nil,

	// UUIDs:
	"uuidv4": nil,

	// SemVer:
	"semver":        nil,
	"semverCompare": nil,

	// Flow Control:
	"fail": nil,

	// Regex
	"regexMatch":                 nil,
	"mustRegexMatch":             nil,
	"regexFindAll":               nil,
	"mustRegexFindAll":           nil,
	"regexFind":                  nil,
	"mustRegexFind":              nil,
	"regexReplaceAll":            nil,
	"mustRegexReplaceAll":        nil,
	"regexReplaceAllLiteral":     nil,
	"mustRegexReplaceAllLiteral": nil,
	"regexSplit":                 nil,
	"mustRegexSplit":             nil,
	"regexQuoteMeta":             nil,

	// URLs:
	"urlParse": nil,
	"urlJoin":  nil,
}
