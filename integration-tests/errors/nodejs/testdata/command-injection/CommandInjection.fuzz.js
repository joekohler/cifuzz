const child_process = require("child_process");

test.fuzz("Test command injection", jazzerBuffer => {
	try {
		child_process.exec(jazzerBuffer.toString());
	} catch (e) {
		// ignore
	}
});
