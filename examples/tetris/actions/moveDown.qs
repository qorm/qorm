# moveDown.qs — the action body; the shared core (fits/refreshView/refreshNext/
# spawn/lock/tickStep) lives in lib.qs.
#
# Soft drop is the gravity step verbatim: one row down, or lock on contact.
if (state.status == "playing") { tickStep() }
