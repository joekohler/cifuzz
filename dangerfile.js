const { danger, warn, message, markdown } = require("danger");
const { basename, dirname } = require("path");

// Check for description
if (!danger.github.pr.body || danger.github.pr.body.length <= 0) {
	warn(
		"This PR doesn't have a description. " +
			"We recommend following the template to include all necessary information."
	);
}

// Check if PR is too big
if (danger.github.pr.changed_files > 15) {
	warn(
		`This PR changes a lot of files (${danger.github.pr.changed_files}). ` +
			"It could be useful to break it up into multiple PRs to keep your changes simple and easy to review."
	);
}

// Checking for missing tests in created files
const createdGoFiles = danger.git.created_files.filter(
	(fileName) => fileName.includes(".go") && !fileName.includes("_test.go")
);
const createdTestFiles = danger.git.created_files.filter((fileName) =>
	fileName.includes("_test.go")
);

const missingTestsForCreatedGoFiles = createdGoFiles.filter(function (x) {
	// Create the test file names for the go file and check
	// if it can be found in the list of created test files
	const filePath = dirname(x);
	const testFile = basename(x).replace(".go", "_test.go");
	return !createdTestFiles.includes(`${filePath}/${testFile}`);
});

// No idea why these extra lines are necessary
// but without them the bullet points *sometimes* don't work
if (missingTestsForCreatedGoFiles?.length > 0) {
	const message = `

  The following created files don't have corresponding test files:
  - ${missingTestsForCreatedGoFiles.join("\n - ")}`;
	warn(message);
}

// Checking for missing tests in modified files
const modifiedGoFiles = danger.git.modified_files.filter(
	(fileName) => fileName.includes(".go") && !fileName.includes("_test.go")
);
const modifiedTestFiles = danger.git.modified_files.filter((fileName) =>
	fileName.includes("_test.go")
);

const missingTestsForModifiedGoFiles = modifiedGoFiles.filter(function (x) {
	// Create the test file names for the go file and check
	// if it can be found in the list of modified or created test files
	const filePath = dirname(x);
	const testFile = basename(x).replace(".go", "_test.go");
	return (
		!modifiedTestFiles.includes(`${filePath}/${testFile}`) &&
		!createdTestFiles.includes(`${filePath}/${testFile}`)
	);
});

if (missingTestsForModifiedGoFiles?.length > 0) {
	const message = `

  The following files have been modified but their tests have not changed:
  - ${missingTestsForModifiedGoFiles.join("\n - ")}`;
	warn(message);
}

// Say thanks if contributors use the template
if (
	danger.github.pr.body?.includes("Motivation/Context") &&
	danger.github.pr.body?.includes("Description") &&
	danger.github.pr.body?.includes("How to use/reproduce")
) {
	markdown("Thank you for using the PR template ❤️");
}

// Say thanks for adding new tests for already existing files
const expandedTestCoverage = createdTestFiles.filter(function (x) {
	// Create the go file names for the test file and check
	// if it can be found in the list of created go files
	const filePath = dirname(x);
	const file = basename(x).replace("_test.go", ".go");
	return !createdGoFiles.includes(`${filePath}/${file}`);
});

if (expandedTestCoverage?.length > 0) {
	markdown(
		"You're a rockstar for creating tests for files that didn't have any yet ⭐"
	);
}

// Congratulate for removing more lines than adding
if (danger.github.pr.deletions > danger.github.pr.additions) {
	markdown("You removed more lines of code than you added, nice cleanup 🧹");
}
