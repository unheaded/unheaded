<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# go-join-trailing

`JoinFields` leaves a trailing separator ("a,b,") and returns "," for a
single empty field list. It must behave like strings.Join for these
inputs. Fix it (you may use strings.Join).
