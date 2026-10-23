<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# go-wrap-error

`LoadConfig` hides the cause of a failure: callers cannot tell a missing
file from any other error. Make the returned error wrap the underlying
one so `errors.Is(err, fs.ErrNotExist)` works. Keep the "load config"
context in the message.
