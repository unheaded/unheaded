-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — index the compliance_host_bindings(profile) foreign key
-- (W-19), unheaded_config. Without it, deleting or renaming a profile scans
-- every binding under lock. Tiny table: no CONCURRENTLY needed.

CREATE INDEX IF NOT EXISTS idx_compliance_host_bindings_profile
    ON compliance_host_bindings (profile);
