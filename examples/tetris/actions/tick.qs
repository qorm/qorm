# tick.qs — the action body; the shared core (fits/refreshView/refreshNext/
# spawn/lock/tickStep) lives in lib.qs.
#
# One gravity step from the timer: slide down when the cells below are free,
# otherwise lock the piece and spawn the next.
if (state.status == "playing") { tickStep() }
