-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — compliance desired state (unheaded_config), ADR-099 amended.
--
-- Profiles and host bindings are the source of truth here; the YAML under
-- compliance/ is their Genesis seed and read-only fallback. Fixes, controls
-- and frameworks are authored in the repository (they run as root or are
-- pinned by tests) and only mirrored here, each with the sha256 of the repo
-- content so a reader can tell a stale or edited mirror from the real one.
-- No new users: config_admin writes, config_reader reads (002/005).

-- ═══════════════════════════════════════════════════════════════════════════════
-- APPEND-ONLY GUARD (history tables: no UPDATE, no DELETE, not even the owner)
-- ═══════════════════════════════════════════════════════════════════════════════
CREATE OR REPLACE FUNCTION compliance_append_only()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

-- ═══════════════════════════════════════════════════════════════════════════════
-- MIRRORS OF REPO-AUTHORED DEFINITIONS (written by `compliance-evidence well sync`)
-- ═══════════════════════════════════════════════════════════════════════════════
CREATE TABLE compliance_fixes (
    id         VARCHAR(40) PRIMARY KEY CHECK (id ~ '^[a-z0-9][a-z0-9-]{0,39}$'),
    title      TEXT NOT NULL,
    class      VARCHAR(20) NOT NULL,
    risk       VARCHAR(10) NOT NULL CHECK (risk IN ('none', 'low', 'medium', 'high')),
    controls   TEXT[] NOT NULL DEFAULT '{}',
    closes     TEXT[] NOT NULL DEFAULT '{}',
    body       JSONB NOT NULL,
    sha256     CHAR(64) NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    synced_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE compliance_controls (
    id         VARCHAR(40) PRIMARY KEY,
    title      TEXT NOT NULL,
    body       JSONB NOT NULL,
    sha256     CHAR(64) NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    synced_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE compliance_frameworks (
    id         VARCHAR(60) PRIMARY KEY,
    title      TEXT NOT NULL,
    body       JSONB NOT NULL,
    sha256     CHAR(64) NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    synced_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ═══════════════════════════════════════════════════════════════════════════════
-- PROFILES (which fixes are on) — source of truth
-- ═══════════════════════════════════════════════════════════════════════════════
CREATE TABLE compliance_profiles (
    name         VARCHAR(40) PRIMARY KEY CHECK (name ~ '^[a-z0-9][a-z0-9-]{0,39}$'),
    description  TEXT NOT NULL DEFAULT '',
    all_fixes    BOOLEAN NOT NULL DEFAULT FALSE,
    fixes        TEXT[] NOT NULL DEFAULT '{}',
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by   TEXT NOT NULL CHECK (length(trim(updated_by)) > 0),
    CHECK (NOT (all_fixes AND cardinality(fixes) > 0))
);

-- An array cannot carry a foreign key: refuse a profile naming a fix the
-- mirror does not have (seed the fixes first).
CREATE OR REPLACE FUNCTION compliance_profile_fixes_exist()
RETURNS TRIGGER AS $$
DECLARE
    missing TEXT;
BEGIN
    SELECT f INTO missing FROM unnest(NEW.fixes) AS f
     WHERE f NOT IN (SELECT id FROM compliance_fixes) LIMIT 1;
    IF missing IS NOT NULL THEN
        RAISE EXCEPTION 'profile % turns on unknown fix %', NEW.name, missing;
    END IF;
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_compliance_profiles_fixes
    BEFORE INSERT OR UPDATE ON compliance_profiles
    FOR EACH ROW EXECUTE FUNCTION compliance_profile_fixes_exist();

-- ═══════════════════════════════════════════════════════════════════════════════
-- HOST BINDINGS (profile + ADR-098 mode per host) — source of truth, every
-- change recorded with who and why (change-management evidence, CM-3)
-- ═══════════════════════════════════════════════════════════════════════════════
CREATE TABLE compliance_host_bindings (
    host           VARCHAR(63) PRIMARY KEY CHECK (host ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
    profile        VARCHAR(40) NOT NULL REFERENCES compliance_profiles(name)
                       ON UPDATE RESTRICT ON DELETE RESTRICT,
    -- Only audit until ADR-098's enforcing gate (LICH-012 run, 7-day soak,
    -- zero host findings, Stevie's sign-off) is met; widening this CHECK is
    -- itself a reviewed migration.
    mode           VARCHAR(10) NOT NULL DEFAULT 'audit' CHECK (mode IN ('audit')),
    overrides      JSONB NOT NULL DEFAULT '{}',
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by     TEXT NOT NULL CHECK (length(trim(updated_by)) > 0),
    change_reason  TEXT NOT NULL CHECK (length(trim(change_reason)) > 0)
);

CREATE TABLE compliance_host_bindings_history (
    id             BIGSERIAL PRIMARY KEY,
    host           VARCHAR(63) NOT NULL,
    old_profile    VARCHAR(40),
    new_profile    VARCHAR(40) NOT NULL,
    old_mode       VARCHAR(10),
    new_mode       VARCHAR(10) NOT NULL,
    old_overrides  JSONB,
    new_overrides  JSONB NOT NULL,
    changed_by     TEXT NOT NULL,
    change_reason  TEXT NOT NULL,
    changed_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_compliance_bindings_history_host ON compliance_host_bindings_history(host, changed_at DESC);

CREATE OR REPLACE FUNCTION compliance_binding_history()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        INSERT INTO compliance_host_bindings_history
            (host, old_profile, new_profile, old_mode, new_mode, old_overrides, new_overrides, changed_by, change_reason)
        VALUES (NEW.host, OLD.profile, NEW.profile, OLD.mode, NEW.mode, OLD.overrides, NEW.overrides,
                NEW.updated_by, NEW.change_reason);
    ELSE
        INSERT INTO compliance_host_bindings_history
            (host, new_profile, new_mode, new_overrides, changed_by, change_reason)
        VALUES (NEW.host, NEW.profile, NEW.mode, NEW.overrides, NEW.updated_by, NEW.change_reason);
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION compliance_binding_touch()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_compliance_bindings_touch
    BEFORE UPDATE ON compliance_host_bindings
    FOR EACH ROW EXECUTE FUNCTION compliance_binding_touch();

CREATE TRIGGER trg_compliance_bindings_history
    AFTER INSERT OR UPDATE ON compliance_host_bindings
    FOR EACH ROW EXECUTE FUNCTION compliance_binding_history();

CREATE TRIGGER trg_compliance_bindings_history_append_only
    BEFORE UPDATE OR DELETE ON compliance_host_bindings_history
    FOR EACH ROW EXECUTE FUNCTION compliance_append_only();

-- A binding is never deleted: a host leaving is a change to profile 'none'.
CREATE TRIGGER trg_compliance_bindings_no_delete
    BEFORE DELETE ON compliance_host_bindings
    FOR EACH ROW EXECUTE FUNCTION compliance_append_only();

-- ═══════════════════════════════════════════════════════════════════════════════
-- GRANTS
-- ═══════════════════════════════════════════════════════════════════════════════
GRANT USAGE, SELECT ON SEQUENCE compliance_host_bindings_history_id_seq TO config_admin;

GRANT SELECT, INSERT, UPDATE, DELETE ON
    compliance_fixes, compliance_controls, compliance_frameworks, compliance_profiles
    TO config_admin;
GRANT SELECT, INSERT, UPDATE ON compliance_host_bindings TO config_admin;
-- History is written by the trigger; config_admin may read it, never edit it.
GRANT SELECT, INSERT ON compliance_host_bindings_history TO config_admin;

GRANT SELECT ON
    compliance_fixes, compliance_controls, compliance_frameworks, compliance_profiles,
    compliance_host_bindings, compliance_host_bindings_history
    TO config_reader;
