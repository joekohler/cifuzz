package summary

import (
	"bufio"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"code-intelligence.com/cifuzz/pkg/log"
)

func count(c *Coverage, key string, value int) {
	switch key {
	case "FNF":
		c.FunctionsFound += value
	case "FNH":
		c.FunctionsHit += value
	case "BRF":
		c.BranchesFound += value
	case "BRH":
		c.BranchesHit += value
	case "LF":
		c.LinesFound += value
	case "LH":
		c.LinesHit += value
	}
}

// ParseLcov takes a lcov tracefile report and turns it into
// the `CoverageSummary` struct. The parsing is as forgiving
// as possible. It will output debug/error logs instead of
// failing, with the goal to gather as much information as
// possible
func ParseLcov(in io.Reader) *CoverageSummary {
	summary := &CoverageSummary{
		Total: &Coverage{},
	}

	var currentFile *FileCoverage

	// The definition of the lcov tracefile format can be viewed
	// with `man geninfo`
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), ":", 2)
		key := parts[0]

		switch key {

		// SF starts a section (for a single file)
		case "SF":
			currentFile = &FileCoverage{
				Filename: parts[1],
				Coverage: &Coverage{},
			}
			summary.Files = append(summary.Files, currentFile)

		// end of a section
		case "end_of_record":
			currentFile = nil

			// high level coverage metrics
		case "FNF", "FNH", "BRF", "BRH", "LF", "LH":
			if len(parts) == 1 {
				log.Debugf("Parsing lcov: no value for key '%s'", key)
				break
			}

			value, err := strconv.Atoi(parts[1])
			if err != nil {
				log.Errorf(err, "Parsing lcov: unable to convert value %s to int: %v", parts[1], err)
				value = 0
			}

			count(summary.Total, key, value)
			if currentFile != nil {
				count(currentFile.Coverage, key, value)
			}

		// these keys are (currently) not relevant for cifuzz
		// so we just ignore them
		case "TN", "FN", "FNDA", "BRDA", "DA":
			log.Debugf("Parsing lcov: Ignored key '%s'. Not implemented by now. ", key)

		// this branch should only be reached if a key shows up
		// that is not defined in the format specification
		default:
			log.Debugf("Parsing lcov: Unknown key '%s'", key)

		}
	}

	out, err := json.MarshalIndent(summary, "", "    ")
	if err != nil {
		log.Errorf(err, "Parsing lcov: Unable to convert coverage summary to json: %v", err)
	} else {
		log.Debugf("Successfully parsed lcov report : %s", string(out))
	}

	return summary
}
