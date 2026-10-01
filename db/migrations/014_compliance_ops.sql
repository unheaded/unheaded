-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — compliance records (unheaded_ops), ADR-098/099 amended.
--
-- What happened, not what is wanted: the apply journal (hash-chained,
-- append-only), findings triage with its history, and evidence records.
-- The YAML register and var/compliance/evidence.json stay as seed and
-- read-only fallback. No new users: ops_writer appends, ops_reader reads.

CREATE OR REPLACE FUNCTION compliance_append_only()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

-- ═══════════════════════════════════════════════════════════════════════════════
-- APPLY JOURNAL — every plan/apply/undo of a fix on a host. Rows arrive from
-- the hosts' local journal.jsonl, pulled by the collector over SSH (The Well
-- stays on loopback). hash = sha256(previous_hash || canonical row), checked
-- on ingest; a broken chain is a critical finding, never repaired.
-- ═══════════════════════════════════════════════════════════════════════════════
CREATE TABLE compliance_apply_journal (
    id             BIGSERIAL PRIMARY KEY,
    host           VARCHAR(63) NOT NULL CHECK (host ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
    host_seq       BIGINT NOT NULL CHECK (host_seq > 0),
    fix_id         VARCHAR(40) NOT NULL,
    fix_sha256     CHAR(64) NOT NULL CHECK (fix_sha256 ~ '^[0-9a-f]{64}$'),
    model_commit   VARCHAR(64) NOT NULL DEFAULT '',
    action         VARCHAR(10) NOT NULL CHECK (action IN ('plan', 'apply', 'undo')),
    outcome        VARCHAR(10) NOT NULL CHECK (outcome IN ('success', 'failure', 'refused', 'restored')),
    backup_dir     TEXT NOT NULL DEFAULT '',
    operator       TEXT NOT NULL CHECK (length(trim(operator)) > 0),
    -- backfilled: written from remediate.sh's state directories after the
    -- fact (2026-10-01 applies), not by the executor at the time.
    backfilled     BOOLEAN NOT NULL DEFAULT FALSE,
    at             TIMESTAMPTZ NOT NULL,
    previous_hash  CHAR(64) NOT NULL CHECK (previous_hash ~ '^[0-9a-f]{64}$'),
    hash           CHAR(64) NOT NULL CHECK (hash ~ '^[0-9a-f]{64}$'),
    ingested_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (host, host_seq),
    UNIQUE (hash)
);

CREATE INDEX idx_compliance_journal_host_fix ON compliance_apply_journal(host, fix_id, at DESC);

CREATE TRIGGER trg_compliance_journal_append_only
    BEFORE UPDATE OR DELETE ON compliance_apply_journal
    FOR EACH ROW EXECUTE FUNCTION compliance_append_only();

-- ═══════════════════════════════════════════════════════════════════════════════
-- FINDINGS TRIAGE — one row per register entry (key = evidence-source key or
-- FND-NNN). entry holds the full RegisterEntry as the Go loader validates it;
-- the columns repeat what is queried and constrained.
-- ═══════════════════════════════════════════════════════════════════════════════
CREATE TABLE compliance_findings (
    key             TEXT PRIMARY KEY CHECK (length(key) BETWEEN 1 AND 300),
    severity        VARCHAR(10) NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low')),
    class           VARCHAR(20) NOT NULL,
    host            VARCHAR(63) NOT NULL DEFAULT '',
    step            INTEGER NOT NULL CHECK (step >= 1),  -- as crosswalk.validateEntry
    lockout         VARCHAR(10) NOT NULL CHECK (lockout IN ('none', 'low', 'high')),
    opened          DATE NOT NULL,
    accepted_on     DATE,
    accepted_until  DATE,
    entry           JSONB NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by      TEXT NOT NULL CHECK (length(trim(updated_by)) > 0),
    change_reason   TEXT NOT NULL CHECK (length(trim(change_reason)) > 0),
    -- ADR-098: an acceptance is capped at 90 days and must be re-decided.
    CHECK ((accepted_on IS NULL) = (accepted_until IS NULL)),
    CHECK (accepted_until IS NULL OR accepted_until <= accepted_on + 90)
);

CREATE INDEX idx_compliance_findings_severity ON compliance_findings(severity);
CREATE INDEX idx_compliance_findings_host ON compliance_findings(host);

CREATE TABLE compliance_findings_history (
    id             BIGSERIAL PRIMARY KEY,
    key            TEXT NOT NULL,
    op             VARCHAR(10) NOT NULL CHECK (op IN ('INSERT', 'UPDATE', 'DELETE')),
    old_entry      JSONB,
    new_entry      JSONB,
    changed_by     TEXT NOT NULL,
    change_reason  TEXT NOT NULL,
    changed_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_compliance_findings_history_key ON compliance_findings_history(key, changed_at DESC);

-- A finding row is deleted when its evidence passes (resolved); the
-- history keeps who removed it and why.
CREATE OR REPLACE FUNCTION compliance_finding_history()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        INSERT INTO compliance_findings_history (key, op, old_entry, changed_by, change_reason)
        VALUES (OLD.key, TG_OP, OLD.entry,
                coalesce(current_setting('compliance.actor', true), current_user),
                coalesce(current_setting('compliance.reason', true), 'resolved'));
        RETURN OLD;
    END IF;
    INSERT INTO compliance_findings_history (key, op, old_entry, new_entry, changed_by, change_reason)
    VALUES (NEW.key, TG_OP, CASE WHEN TG_OP = 'UPDATE' THEN OLD.entry END, NEW.entry,
            NEW.updated_by, NEW.change_reason);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION compliance_finding_touch()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_compliance_findings_touch
    BEFORE UPDATE ON compliance_findings
    FOR EACH ROW EXECUTE FUNCTION compliance_finding_touch();

CREATE TRIGGER trg_compliance_findings_history
    AFTER INSERT OR UPDATE OR DELETE ON compliance_findings
    FOR EACH ROW EXECUTE FUNCTION compliance_finding_history();

CREATE TRIGGER trg_compliance_findings_history_append_only
    BEFORE UPDATE OR DELETE ON compliance_findings_history
    FOR EACH ROW EXECUTE FUNCTION compliance_append_only();

-- ═══════════════════════════════════════════════════════════════════════════════
-- EVIDENCE — collector records (crosswalk.Record). Readers take the latest per
-- source; old rows age out by retention policy, never by edit.
-- ═══════════════════════════════════════════════════════════════════════════════
CREATE TABLE compliance_evidence (
    id           BIGSERIAL PRIMARY KEY,
    source_key   TEXT NOT NULL CHECK (length(source_key) BETWEEN 1 AND 300),
    verdict      VARCHAR(10) NOT NULL CHECK (verdict IN ('pass', 'fail')),
    observed_at  TIMESTAMPTZ NOT NULL,
    commit       VARCHAR(64) NOT NULL DEFAULT '',
    detail       TEXT NOT NULL DEFAULT '' CHECK (length(detail) <= 4000),
    ingested_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_compliance_evidence_latest ON compliance_evidence(source_key, observed_at DESC);

CREATE TRIGGER trg_compliance_evidence_no_update
    BEFORE UPDATE ON compliance_evidence
    FOR EACH ROW EXECUTE FUNCTION compliance_append_only();

-- ═══════════════════════════════════════════════════════════════════════════════
-- GRANTS
-- ═══════════════════════════════════════════════════════════════════════════════
GRANT USAGE, SELECT ON SEQUENCE
    compliance_apply_journal_id_seq, compliance_findings_history_id_seq, compliance_evidence_id_seq
    TO ops_writer;

-- Journal and evidence: append only. Findings: triage may change and
-- resolve rows; their history is trigger-written and read-only.
GRANT SELECT, INSERT ON compliance_apply_journal, compliance_evidence TO ops_writer;
GRANT SELECT, INSERT, UPDATE, DELETE ON compliance_findings TO ops_writer;
GRANT SELECT, INSERT ON compliance_findings_history TO ops_writer;

GRANT SELECT ON
    compliance_apply_journal, compliance_findings, compliance_findings_history, compliance_evidence
    TO ops_reader;
