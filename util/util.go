package util

import (

	//	"os/exec"
	"bufio"
	"crypto/rand"
	"io"
	rando "math/rand"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	log "github.com/sirupsen/logrus"
)

const (
	randomStringOptions = "abcdefghijklmnopqrstuvwxyz0123456789"
	randomStringLength  = 6
)

// newLogWriter returns an io.Writer that logs each full line written to it to the provided logrus
// Entry.
func NewLogWriter(l *log.Entry) io.Writer {
	r, w := io.Pipe()
	s := bufio.NewScanner(r)
	go func() {
		for s.Scan() {
			l.Info(s.Text())
		}
		if err := s.Err(); err != nil {
			l.WithError(err).Error("error scanning lines")
		}
	}()
	return w
}

func Name() string {
	return generateName() + "-" + randomString(randomStringLength)
}

func ToPulumi(v any) pulumi.Input {
	var m = make(pulumi.Map)
	if v == nil {
		log.Info("Type is nil")
		return nil
	}
	switch v := v.(type) {
	case string:
		return pulumi.String(v)
	case bool:
		return pulumi.Bool(v)
	case int:
		return pulumi.Int(v)
	case float64:
		return pulumi.Float64(v)
	case map[string]any:
		for k, vv := range v {
			m[k] = ToPulumi(vv)
		}
	case []any:
		var s pulumi.MapArray
		for _, vv := range v {
			s = append(s, ToPulumi(vv).(pulumi.MapInput))
		}
		return s
	default:
		log.Errorf("Type not handled i=%s", v)
	}
	return m
}

// TODO: Write tests
func MergeMaps(src, dest map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	for k, v := range dest {
		if v, ok := v.(map[string]any); ok {
			if destv, ok := out[k]; ok {
				if destv, ok := destv.(map[string]any); ok {
					out[k] = MergeMaps(destv, v)
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}

func randomString(n int) string {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = randomStringOptions[b[i]%byte(len(randomStringOptions))]
	}
	return string(b)
}

func generateName() string {
	var WORKSPACE_PACHYDERMS = []string{
		"alpaca",
		"antelope",
		"bison",
		"boar",
		"buffalo",
		"camel",
		"caribou",
		"cow",
		"deer",
		"donkey",
		"dugong",
		"elephant",
		"elk",
		"entelodont",
		"giraffes",
		"gnu",
		"goats",
		"hippopotamus",
		"horse",
		"hyrax",
		"javelina",
		"kiang",
		"llama",
		"mammoth",
		"manatee",
		"mastodon",
		"mule",
		"onager",
		"peccary",
		"pig",
		"rhinoceros",
		"sheep",
		"sivatherium",
		"stegodon",
		"tapir",
		"warthog",
		"zebra",
	}

	var WORKSPACE_ADJECTIVES = []string{
		"admiring",
		"adorable",
		"adoring",
		"agitated",
		"amazing",
		"ancient",
		"available",
		"awesome",
		"big",
		"brainy",
		"burrowing",
		"busy",
		"calm",
		"clever",
		"colorful",
		"compassionate",
		"consistent",
		"cuddly",
		"delightful",
		"determined",
		"diamond",
		"didactic",
		"dreamy",
		"eager",
		"eccentric",
		"ecstatic",
		"elastic",
		"elated",
		"elegant",
		"eloquent",
		"energetic",
		"fancy",
		"fast",
		"fastidious",
		"flamboyant",
		"flying",
		"focused",
		"friendly",
		"fuzzy",
		"gigantic",
		"goofy",
		"graceful",
		"happy",
		"hopeful",
		"hungry",
		"idempotent",
		"immutable",
		"insured",
		"intrepid",
		"jolly",
		"jovial",
		"large",
		"lavish",
		"lively",
		"lonely",
		"loving",
		"loyal",
		"medium",
		"mithril",
		"modest",
		"mutable",
		"neutral",
		"noisy",
		"nostalgic",
		"oblivious",
		"overdressed",
		"pleasant",
		"powerful",
		"resistant",
		"reverent",
		"romantic",
		"serendipitous",
		"serene",
		"sharp",
		"silly",
		"sleepy",
		"small",
		"sparkling",
		"stoic",
		"suspicious",
		"tame",
		"thirsty",
		"timeless",
		"tiny",
		"tolerant",
		"unbounded",
		"underdressed",
		"whimsical",
		"wild",
	}

	return WORKSPACE_ADJECTIVES[rando.Intn(len(WORKSPACE_ADJECTIVES))] + "-" + WORKSPACE_PACHYDERMS[rando.Intn(len(WORKSPACE_PACHYDERMS))]
}
