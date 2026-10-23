// SPDX-License-Identifier: GPL-3.0-or-later

package fx

import "unheaded/pkg/crypto/pqc"

// The in-tree PQC wrapper, used for ML-DSA-65 (as Wotan topic signing does).
var L = pqc.MLDSA65
