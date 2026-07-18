package claude

import "syscall"

// sigInterrupt is the signal sent to the claude process on Interrupt.
// SIGINT allows the CLI to flush buffered output; the caller falls back to a
// hard kill (SIGKILL) if the signal cannot be delivered.
var sigInterrupt = syscall.SIGINT
