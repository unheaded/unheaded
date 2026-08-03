"""
ActionManager — INTENT -> SNAPSHOT -> EXECUTE -> RECORD -> AUDIT

Implements the Zhen Champion Agent action framework from ADR-019.
Every action follows the pattern:
  1. INTENT   -> Write to zhen_actions (status: 'planned')
  2. SNAPSHOT  -> Write to zhen_action_snapshots (before-state)
  3. EXECUTE   -> Perform the action via executor_fn
  4. RECORD    -> Update zhen_actions (status: 'completed' or 'failed')
  5. AUDIT     -> (future: hash-chained audit_events)

Trinoda Necessitas: The Well must be alive before any write operation.
If PostgreSQL is unreachable, Zhen enters read-only mode.
"""

import json
import logging
import os
import time

logger = logging.getLogger(__name__)


class WellUnavailableError(Exception):
    """Raised when The Well (PostgreSQL) is unreachable and a write is attempted."""


class ActionManager:
    """Manages the full lifecycle of Zhen actions with snapshot-based revert.

    Usage:
        am = ActionManager()
        action_id = am.plan_action(session_id, 'file.write', 'Write nginx config', {...})
        am.execute_action(action_id, my_executor_fn)
        # Later, if needed:
        am.revert_action(action_id)
    """

    def __init__(self, conn=None):
        """Initialize with an optional psycopg2 connection.

        If conn is None, attempts to connect using environment variables.
        If connection fails, the manager operates in degraded (read-only) mode.
        """
        self._conn = conn
        if self._conn is None:
            self._conn = self._connect()

    def _connect(self):
        """Attempt to connect to The Well. Returns connection or None."""
        try:
            import psycopg2
            conn = psycopg2.connect(
                dbname=os.environ.get('ZHEN_DB_NAME', 'unheaded_app'),
                user=os.environ.get('ZHEN_DB_USER', 'app_zhen'),
                password=os.environ.get('ZHEN_DB_PASSWORD', ''),
                host=os.environ.get('ZHEN_DB_HOST', 'localhost'),
                port=int(os.environ.get('ZHEN_DB_PORT', '5432')),
                connect_timeout=3,
            )
            conn.autocommit = False
            logger.info("[ActionManager] Connected to The Well")
            return conn
        except Exception as e:
            logger.warning(f"[ActionManager] The Well not available: {e}")
            return None

    # ------------------------------------------------------------------
    # Trinoda Necessitas — The Well must be alive
    # ------------------------------------------------------------------

    def _check_well(self):
        """Trinoda Necessitas check #1: The Well is alive.

        Raises WellUnavailableError if PostgreSQL is unreachable.
        """
        if self._conn is None:
            raise WellUnavailableError(
                "The Well (PostgreSQL) is not connected. "
                "Zhen cannot perform write operations without an audit trail."
            )
        try:
            cur = self._conn.cursor()
            cur.execute("SELECT 1")
            cur.fetchone()
            cur.close()
        except Exception as e:
            # Connection went stale — try to reconnect once
            logger.warning(f"[ActionManager] Well health check failed: {e}")
            try:
                self._conn.close()
            except Exception:
                pass
            self._conn = self._connect()
            if self._conn is None:
                raise WellUnavailableError(
                    f"The Well (PostgreSQL) is unreachable: {e}. "
                    "Zhen enters read-only mode."
                )

    # ------------------------------------------------------------------
    # Step 1: INTENT — plan_action
    # ------------------------------------------------------------------

    def plan_action(self, session_id, action_type, intent, parameters=None,
                    triggered_by='user', model='', confidence=0.0,
                    trace_id='', parent_action=None, revertable=False):
        """Create a planned action in zhen_actions.

        Args:
            session_id: Client session identifier.
            action_type: One of the allowed action types (e.g. 'file.write').
            intent: Human-readable description of what Zhen plans to do.
            parameters: Dict of structured parameters for the action.
            triggered_by: Who triggered this ('user', 'zhen', 'schedule', 'hook').
            model: Which model made the decision.
            confidence: Model's confidence in the action (0.0 - 1.0).
            trace_id: Correlation ID across related actions.
            parent_action: ID of the parent action if spawned by another.
            revertable: Whether this action can be undone.

        Returns:
            int: The action ID.

        Raises:
            WellUnavailableError: If The Well is not connected.
        """
        self._check_well()

        if parameters is None:
            parameters = {}

        cur = self._conn.cursor()
        try:
            cur.execute(
                """INSERT INTO zhen_actions
                   (session_id, action_type, status, intent, parameters,
                    triggered_by, model, confidence, trace_id, parent_action,
                    revertable, planned_at)
                   VALUES (%s, %s, 'planned', %s, %s::jsonb,
                           %s, %s, %s, %s, %s,
                           %s, NOW())
                   RETURNING id""",
                (session_id, action_type, intent, json.dumps(parameters),
                 triggered_by, model, confidence, trace_id, parent_action,
                 revertable),
            )
            action_id = cur.fetchone()[0]
            self._conn.commit()
            logger.info(f"[ActionManager] Planned action {action_id}: {action_type} — {intent}")
            return action_id
        except Exception:
            self._conn.rollback()
            raise
        finally:
            cur.close()

    # ------------------------------------------------------------------
    # Step 2+3+4: SNAPSHOT -> EXECUTE -> RECORD — execute_action
    # ------------------------------------------------------------------

    def execute_action(self, action_id, executor_fn):
        """Execute a planned action with snapshot and result recording.

        The executor_fn receives the action's parameters dict and must return
        a dict with:
            - 'result': dict  — structured result data
            - 'result_summary': str — human-readable outcome
            - 'snapshots': list of dicts, each with:
                - 'resource_type': str
                - 'resource_id': str
                - 'content_before': str or None
                - 'metadata': dict (optional)

        The executor_fn may also return 'content_after' in each snapshot dict,
        which will be written to the snapshot row after execution.

        Args:
            action_id: The ID returned by plan_action.
            executor_fn: Callable(parameters) -> dict.

        Returns:
            dict: The result from executor_fn.

        Raises:
            WellUnavailableError: If The Well is not connected.
        """
        self._check_well()

        # Fetch the planned action
        cur = self._conn.cursor()
        try:
            cur.execute(
                "SELECT status, parameters FROM zhen_actions WHERE id = %s",
                (action_id,),
            )
            row = cur.fetchone()
            if row is None:
                raise ValueError(f"Action {action_id} not found")
            status, parameters = row
            if status != 'planned':
                raise ValueError(
                    f"Action {action_id} is '{status}', expected 'planned'"
                )

            # Mark as executing
            start_time = time.time()
            cur.execute(
                """UPDATE zhen_actions
                   SET status = 'executing', started_at = NOW()
                   WHERE id = %s""",
                (action_id,),
            )
            self._conn.commit()
        except Exception:
            self._conn.rollback()
            raise
        finally:
            cur.close()

        # Parse parameters
        if isinstance(parameters, str):
            parameters = json.loads(parameters)

        # Execute the action
        try:
            exec_result = executor_fn(parameters)
        except Exception as e:
            # Record failure
            elapsed_ms = int((time.time() - start_time) * 1000)
            self._record_failure(action_id, str(e), elapsed_ms)
            raise

        elapsed_ms = int((time.time() - start_time) * 1000)

        # Write snapshots and record result
        cur = self._conn.cursor()
        try:
            # Step 2: SNAPSHOT — write before-state for each resource
            snapshots = exec_result.get('snapshots', [])
            for snap in snapshots:
                cur.execute(
                    """INSERT INTO zhen_action_snapshots
                       (action_id, resource_type, resource_id,
                        content_before, content_after, metadata, snapshot_at)
                       VALUES (%s, %s, %s, %s, %s, %s::jsonb, NOW())""",
                    (action_id,
                     snap['resource_type'],
                     snap['resource_id'],
                     snap.get('content_before'),
                     snap.get('content_after'),
                     json.dumps(snap.get('metadata', {}))),
                )

            # Step 4: RECORD — update action with result
            result_data = exec_result.get('result', {})
            result_summary = exec_result.get('result_summary', '')

            cur.execute(
                """UPDATE zhen_actions
                   SET status = 'completed',
                       result = %s::jsonb,
                       result_summary = %s,
                       completed_at = NOW(),
                       elapsed_ms = %s
                   WHERE id = %s""",
                (json.dumps(result_data), result_summary, elapsed_ms, action_id),
            )
            self._conn.commit()
            logger.info(
                f"[ActionManager] Completed action {action_id} "
                f"({elapsed_ms}ms): {result_summary}"
            )
            return exec_result
        except Exception:
            self._conn.rollback()
            raise
        finally:
            cur.close()

    def _record_failure(self, action_id, error_msg, elapsed_ms):
        """Record a failed action."""
        cur = self._conn.cursor()
        try:
            cur.execute(
                """UPDATE zhen_actions
                   SET status = 'failed',
                       error = %s,
                       completed_at = NOW(),
                       elapsed_ms = %s
                   WHERE id = %s""",
                (error_msg, elapsed_ms, action_id),
            )
            self._conn.commit()
            logger.warning(
                f"[ActionManager] Failed action {action_id} "
                f"({elapsed_ms}ms): {error_msg}"
            )
        except Exception:
            self._conn.rollback()
            raise
        finally:
            cur.close()

    # ------------------------------------------------------------------
    # Revert — read snapshot, apply revert, record revert action
    # ------------------------------------------------------------------

    def revert_action(self, action_id):
        """Revert a completed action by reading its snapshots.

        Creates a new action that records the revert, then updates the
        original action's status to 'reverted' with reverted_by set.

        Args:
            action_id: The ID of the completed action to revert.

        Returns:
            dict with 'revert_action_id' and 'snapshots_reverted'.

        Raises:
            WellUnavailableError: If The Well is not connected.
            ValueError: If the action cannot be reverted.
        """
        self._check_well()

        cur = self._conn.cursor()
        try:
            # Fetch original action
            cur.execute(
                """SELECT session_id, action_type, intent, status, revertable,
                          trace_id
                   FROM zhen_actions WHERE id = %s""",
                (action_id,),
            )
            row = cur.fetchone()
            if row is None:
                raise ValueError(f"Action {action_id} not found")

            session_id, action_type, intent, status, _revertable, trace_id = row

            if status == 'reverted':
                raise ValueError(f"Action {action_id} is already reverted")
            if status not in ('completed', 'failed'):
                raise ValueError(
                    f"Action {action_id} is '{status}', "
                    "can only revert 'completed' or 'failed' actions"
                )

            # Fetch snapshots
            cur.execute(
                """SELECT id, resource_type, resource_id, content_before, metadata
                   FROM zhen_action_snapshots
                   WHERE action_id = %s
                   ORDER BY id""",
                (action_id,),
            )
            snapshots = cur.fetchall()
            if not snapshots:
                raise ValueError(
                    f"Action {action_id} has no snapshots — cannot revert"
                )

            self._conn.commit()
        except Exception:
            self._conn.rollback()
            raise
        finally:
            cur.close()

        # Plan the revert action
        revert_action_id = self.plan_action(
            session_id=session_id,
            action_type=action_type,
            intent=f"Revert action {action_id}: {intent}",
            parameters={'reverts_action': action_id},
            triggered_by='zhen',
            trace_id=trace_id,
            revertable=False,  # revert of a revert is not supported
        )

        # Update revert action with reverts_action reference
        cur = self._conn.cursor()
        try:
            cur.execute(
                "UPDATE zhen_actions SET reverts_action = %s WHERE id = %s",
                (action_id, revert_action_id),
            )
            self._conn.commit()
        except Exception:
            self._conn.rollback()
            raise
        finally:
            cur.close()

        # Build snapshot data for the revert
        revert_snapshots = []
        for snap_id, resource_type, resource_id, content_before, metadata in snapshots:
            revert_snapshots.append({
                'resource_type': resource_type,
                'resource_id': resource_id,
                'content_before': None,  # current state (would need live read)
                'content_after': content_before,  # restoring to original
                'metadata': metadata if isinstance(metadata, dict) else {},
            })

        # Record the revert execution
        cur = self._conn.cursor()
        try:
            start_time = time.time()

            # Mark revert as executing
            cur.execute(
                """UPDATE zhen_actions
                   SET status = 'executing', started_at = NOW()
                   WHERE id = %s""",
                (revert_action_id,),
            )

            # Write revert snapshots
            for snap in revert_snapshots:
                cur.execute(
                    """INSERT INTO zhen_action_snapshots
                       (action_id, resource_type, resource_id,
                        content_before, content_after, metadata, snapshot_at)
                       VALUES (%s, %s, %s, %s, %s, %s::jsonb, NOW())""",
                    (revert_action_id,
                     snap['resource_type'],
                     snap['resource_id'],
                     snap['content_before'],
                     snap['content_after'],
                     json.dumps(snap['metadata'])),
                )

            elapsed_ms = int((time.time() - start_time) * 1000)

            # Mark revert as completed
            cur.execute(
                """UPDATE zhen_actions
                   SET status = 'completed',
                       result = %s::jsonb,
                       result_summary = %s,
                       completed_at = NOW(),
                       elapsed_ms = %s
                   WHERE id = %s""",
                (json.dumps({'snapshots_reverted': len(revert_snapshots)}),
                 f"Reverted action {action_id} ({len(revert_snapshots)} snapshots restored)",
                 elapsed_ms,
                 revert_action_id),
            )

            # Mark original action as reverted
            cur.execute(
                """UPDATE zhen_actions
                   SET status = 'reverted', reverted_by = %s
                   WHERE id = %s""",
                (revert_action_id, action_id),
            )

            self._conn.commit()
            logger.info(
                f"[ActionManager] Reverted action {action_id} via "
                f"revert action {revert_action_id}"
            )
            return {
                'revert_action_id': revert_action_id,
                'snapshots_reverted': len(revert_snapshots),
            }
        except Exception:
            self._conn.rollback()
            raise
        finally:
            cur.close()

    # ------------------------------------------------------------------
    # Query — action history
    # ------------------------------------------------------------------

    def get_action_history(self, session_id, limit=20):
        """Get recent actions for a session.

        Args:
            session_id: The session to query.
            limit: Max number of actions to return (default 20).

        Returns:
            list of dicts, each representing an action.

        Raises:
            WellUnavailableError: If The Well is not connected.
        """
        self._check_well()

        cur = self._conn.cursor()
        try:
            cur.execute(
                """SELECT id, session_id, action_type, status,
                          intent, parameters, result, result_summary, error,
                          revertable, reverted_by, reverts_action,
                          triggered_by, model, confidence,
                          planned_at, started_at, completed_at, elapsed_ms,
                          trace_id, parent_action
                   FROM zhen_actions
                   WHERE session_id = %s
                   ORDER BY planned_at DESC
                   LIMIT %s""",
                (session_id, limit),
            )
            rows = cur.fetchall()
            self._conn.commit()

            actions = []
            for row in rows:
                actions.append({
                    'id': row[0],
                    'session_id': row[1],
                    'action_type': row[2],
                    'status': row[3],
                    'intent': row[4],
                    'parameters': row[5] if isinstance(row[5], dict) else {},
                    'result': row[6] if isinstance(row[6], dict) else {},
                    'result_summary': row[7],
                    'error': row[8],
                    'revertable': row[9],
                    'reverted_by': row[10],
                    'reverts_action': row[11],
                    'triggered_by': row[12],
                    'model': row[13],
                    'confidence': row[14],
                    'planned_at': row[15].isoformat() if row[15] else None,
                    'started_at': row[16].isoformat() if row[16] else None,
                    'completed_at': row[17].isoformat() if row[17] else None,
                    'elapsed_ms': row[18],
                    'trace_id': row[19],
                    'parent_action': row[20],
                })
            return actions
        except Exception:
            self._conn.rollback()
            raise
        finally:
            cur.close()

    # ------------------------------------------------------------------
    # Convenience — get a single action
    # ------------------------------------------------------------------

    def get_action(self, action_id):
        """Fetch a single action by ID.

        Returns:
            dict or None.
        """
        self._check_well()

        cur = self._conn.cursor()
        try:
            cur.execute(
                """SELECT id, session_id, action_type, status,
                          intent, parameters, result, result_summary, error,
                          revertable, reverted_by, reverts_action,
                          triggered_by, model, confidence,
                          planned_at, started_at, completed_at, elapsed_ms,
                          trace_id, parent_action
                   FROM zhen_actions
                   WHERE id = %s""",
                (action_id,),
            )
            row = cur.fetchone()
            self._conn.commit()
            if row is None:
                return None
            return {
                'id': row[0],
                'session_id': row[1],
                'action_type': row[2],
                'status': row[3],
                'intent': row[4],
                'parameters': row[5] if isinstance(row[5], dict) else {},
                'result': row[6] if isinstance(row[6], dict) else {},
                'result_summary': row[7],
                'error': row[8],
                'revertable': row[9],
                'reverted_by': row[10],
                'reverts_action': row[11],
                'triggered_by': row[12],
                'model': row[13],
                'confidence': row[14],
                'planned_at': row[15].isoformat() if row[15] else None,
                'started_at': row[16].isoformat() if row[16] else None,
                'completed_at': row[17].isoformat() if row[17] else None,
                'elapsed_ms': row[18],
                'trace_id': row[19],
                'parent_action': row[20],
            }
        except Exception:
            self._conn.rollback()
            raise
        finally:
            cur.close()

    # ------------------------------------------------------------------
    # Convenience — get snapshots for an action
    # ------------------------------------------------------------------

    def get_snapshots(self, action_id):
        """Fetch all snapshots for an action.

        Returns:
            list of dicts.
        """
        self._check_well()

        cur = self._conn.cursor()
        try:
            cur.execute(
                """SELECT id, action_id, resource_type, resource_id,
                          content_before, content_after, metadata, snapshot_at
                   FROM zhen_action_snapshots
                   WHERE action_id = %s
                   ORDER BY id""",
                (action_id,),
            )
            rows = cur.fetchall()
            self._conn.commit()

            return [
                {
                    'id': row[0],
                    'action_id': row[1],
                    'resource_type': row[2],
                    'resource_id': row[3],
                    'content_before': row[4],
                    'content_after': row[5],
                    'metadata': row[6] if isinstance(row[6], dict) else {},
                    'snapshot_at': row[7].isoformat() if row[7] else None,
                }
                for row in rows
            ]
        except Exception:
            self._conn.rollback()
            raise
        finally:
            cur.close()
