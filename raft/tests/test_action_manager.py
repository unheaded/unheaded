#!/usr/bin/env python3
"""
Tests for ActionManager — Zhen Champion Agent action framework.

Uses an in-memory SQLite-style approach with a real PostgreSQL connection
when available, falling back to a mock connection for CI/unit testing.

Test categories:
  1. Plan/execute/revert workflow
  2. Snapshots are append-only
  3. Trinoda Necessitas (fail gracefully when DB is down)
"""

import json
import os
import sys
import unittest
from datetime import datetime, timezone

# Add scripts dir to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'scripts'))
from action_manager import ActionManager, WellUnavailableError


class MockCursor:
    """Mock psycopg2 cursor that simulates zhen_actions and zhen_action_snapshots."""

    def __init__(self, db):
        self._db = db
        self._last_query = ''
        self._last_params = ()
        self._result = None
        self._results = []

    def execute(self, query, params=None):
        # Normalize whitespace for pattern matching
        self._last_query = ' '.join(query.split())
        self._last_params = params or ()
        query = self._last_query

        if 'SELECT 1' in query:
            self._result = (1,)
            return

        if 'INSERT INTO zhen_actions' in query:
            self._db['action_seq'] += 1
            action_id = self._db['action_seq']
            # Parse params based on position
            action = {
                'id': action_id,
                'session_id': params[0],
                'action_type': params[1],
                'status': 'planned',
                'intent': params[2],
                'parameters': json.loads(params[3]) if isinstance(params[3], str) else params[3],
                'result': {},
                'result_summary': '',
                'error': '',
                'revertable': params[9] if len(params) > 9 else False,
                'reverted_by': None,
                'reverts_action': None,
                'triggered_by': params[4] if len(params) > 4 else 'user',
                'model': params[5] if len(params) > 5 else '',
                'confidence': params[6] if len(params) > 6 else 0.0,
                'trace_id': params[7] if len(params) > 7 else '',
                'parent_action': params[8] if len(params) > 8 else None,
                'planned_at': datetime.now(timezone.utc),
                'started_at': None,
                'completed_at': None,
                'elapsed_ms': 0,
            }
            self._db['actions'][action_id] = action
            self._result = (action_id,)
            return

        if 'INSERT INTO zhen_action_snapshots' in query:
            self._db['snapshot_seq'] += 1
            snap_id = self._db['snapshot_seq']
            snapshot = {
                'id': snap_id,
                'action_id': params[0],
                'resource_type': params[1],
                'resource_id': params[2],
                'content_before': params[3],
                'content_after': params[4],
                'metadata': json.loads(params[5]) if isinstance(params[5], str) else params[5],
                'snapshot_at': datetime.now(timezone.utc),
            }
            self._db['snapshots'].append(snapshot)
            self._result = (snap_id,)
            return

        if 'UPDATE zhen_actions' in query:
            # Find the action_id (last param)
            action_id = params[-1]
            action = self._db['actions'].get(action_id)
            if action:
                if 'status = \'executing\'' in query:
                    action['status'] = 'executing'
                    action['started_at'] = datetime.now(timezone.utc)
                elif 'status = \'completed\'' in query:
                    action['status'] = 'completed'
                    action['result'] = json.loads(params[0]) if isinstance(params[0], str) else params[0]
                    action['result_summary'] = params[1]
                    action['completed_at'] = datetime.now(timezone.utc)
                    action['elapsed_ms'] = params[2]
                elif 'status = \'failed\'' in query:
                    action['status'] = 'failed'
                    action['error'] = params[0]
                    action['elapsed_ms'] = params[1]
                    action['completed_at'] = datetime.now(timezone.utc)
                elif 'status = \'reverted\'' in query:
                    action['status'] = 'reverted'
                    action['reverted_by'] = params[0]
                elif 'reverts_action' in query:
                    action['reverts_action'] = params[0]
            return

        if 'FROM zhen_actions WHERE id' in query:
            action_id = params[0]
            action = self._db['actions'].get(action_id)
            if action:
                if 'status, parameters' in query:
                    self._result = (action['status'], json.dumps(action['parameters']))
                elif 'session_id, action_type, intent' in query:
                    self._result = (
                        action['session_id'], action['action_type'],
                        action['intent'], action['status'],
                        action['revertable'], action['trace_id'],
                    )
                else:
                    self._result = self._action_to_full_row(action)
            else:
                self._result = None
            return

        if 'FROM zhen_actions' in query and 'session_id' in query:
            session_id = params[0]
            limit = params[1] if len(params) > 1 else 20
            matching = [
                a for a in self._db['actions'].values()
                if a['session_id'] == session_id
            ]
            matching.sort(key=lambda a: a['planned_at'] or datetime.min.replace(tzinfo=timezone.utc), reverse=True)
            self._results = [self._action_to_full_row(a) for a in matching[:limit]]
            return

        if 'FROM zhen_action_snapshots' in query:
            action_id = params[0]
            matching = [s for s in self._db['snapshots'] if s['action_id'] == action_id]
            matching.sort(key=lambda s: s['id'])
            if 'content_before, metadata' in query:
                # Revert query — 5 columns
                self._results = [
                    (s['id'], s['resource_type'], s['resource_id'],
                     s['content_before'], s['metadata'])
                    for s in matching
                ]
            else:
                self._results = [
                    (s['id'], s['action_id'], s['resource_type'], s['resource_id'],
                     s['content_before'], s['content_after'],
                     s['metadata'], s['snapshot_at'])
                    for s in matching
                ]
            return

    def _action_to_full_row(self, a):
        return (
            a['id'], a['session_id'], a['action_type'], a['status'],
            a['intent'], a['parameters'], a['result'], a['result_summary'],
            a['error'], a['revertable'], a['reverted_by'], a['reverts_action'],
            a['triggered_by'], a['model'], a['confidence'],
            a['planned_at'], a['started_at'], a['completed_at'], a['elapsed_ms'],
            a['trace_id'], a['parent_action'],
        )

    def fetchone(self):
        return self._result

    def fetchall(self):
        return self._results

    def close(self):
        pass


class MockConnection:
    """Mock psycopg2 connection backed by in-memory dicts."""

    def __init__(self):
        self._db = {
            'actions': {},
            'snapshots': [],
            'action_seq': 0,
            'snapshot_seq': 0,
        }
        self.autocommit = False
        self._closed = False

    def cursor(self):
        if self._closed:
            raise Exception("connection is closed")
        return MockCursor(self._db)

    def commit(self):
        pass

    def rollback(self):
        pass

    def close(self):
        self._closed = True


class DeadConnection:
    """Mock connection that always raises on cursor()."""

    def cursor(self):
        raise Exception("connection refused")

    def commit(self):
        raise Exception("connection refused")

    def rollback(self):
        pass

    def close(self):
        pass


# ======================================================================
# Test cases
# ======================================================================


class TestPlanAction(unittest.TestCase):
    """Test action planning (INTENT step)."""

    def setUp(self):
        self.conn = MockConnection()
        self.am = ActionManager(conn=self.conn)

    def test_plan_creates_action(self):
        action_id = self.am.plan_action(
            session_id='test-session',
            action_type='file.write',
            intent='Write nginx config for sophia',
            parameters={'path': '/etc/nginx/sophia.conf', 'content': 'server {}'},
        )
        self.assertEqual(action_id, 1)
        action = self.conn._db['actions'][1]
        self.assertEqual(action['status'], 'planned')
        self.assertEqual(action['action_type'], 'file.write')
        self.assertEqual(action['intent'], 'Write nginx config for sophia')
        self.assertEqual(action['parameters']['path'], '/etc/nginx/sophia.conf')

    def test_plan_with_provenance(self):
        action_id = self.am.plan_action(
            session_id='s1',
            action_type='kanban.create',
            intent='Create task for drift fix',
            triggered_by='zhen',
            model='mistral-7b',
            confidence=0.85,
            trace_id='trace-abc',
        )
        action = self.conn._db['actions'][action_id]
        self.assertEqual(action['triggered_by'], 'zhen')
        self.assertEqual(action['model'], 'mistral-7b')
        self.assertAlmostEqual(action['confidence'], 0.85)
        self.assertEqual(action['trace_id'], 'trace-abc')

    def test_plan_default_parameters(self):
        action_id = self.am.plan_action(
            session_id='s1',
            action_type='service.health_check',
            intent='Check wotan health',
        )
        action = self.conn._db['actions'][action_id]
        self.assertEqual(action['parameters'], {})

    def test_plan_increments_ids(self):
        id1 = self.am.plan_action('s1', 'file.read', 'Read file A')
        id2 = self.am.plan_action('s1', 'file.read', 'Read file B')
        self.assertEqual(id1, 1)
        self.assertEqual(id2, 2)


class TestExecuteAction(unittest.TestCase):
    """Test the SNAPSHOT -> EXECUTE -> RECORD pattern."""

    def setUp(self):
        self.conn = MockConnection()
        self.am = ActionManager(conn=self.conn)

    def test_execute_success(self):
        action_id = self.am.plan_action(
            session_id='s1',
            action_type='file.write',
            intent='Write config',
            parameters={'path': '/tmp/test.conf', 'content': 'new content'},
        )

        def executor(params):
            return {
                'result': {'bytes_written': 42},
                'result_summary': 'Wrote 42 bytes to /tmp/test.conf',
                'snapshots': [{
                    'resource_type': 'file',
                    'resource_id': '/tmp/test.conf',
                    'content_before': 'old content',
                    'content_after': 'new content',
                    'metadata': {'permissions': '0644'},
                }],
            }

        result = self.am.execute_action(action_id, executor)

        # Check action status
        action = self.conn._db['actions'][action_id]
        self.assertEqual(action['status'], 'completed')
        self.assertEqual(action['result']['bytes_written'], 42)
        self.assertIn('42 bytes', action['result_summary'])

        # Check snapshot was written
        snapshots = [s for s in self.conn._db['snapshots'] if s['action_id'] == action_id]
        self.assertEqual(len(snapshots), 1)
        self.assertEqual(snapshots[0]['content_before'], 'old content')
        self.assertEqual(snapshots[0]['content_after'], 'new content')
        self.assertEqual(snapshots[0]['resource_type'], 'file')

    def test_execute_failure_records_error(self):
        action_id = self.am.plan_action('s1', 'file.write', 'Write config')

        def failing_executor(params):
            raise RuntimeError("Permission denied: /etc/nginx.conf")

        with self.assertRaises(RuntimeError):
            self.am.execute_action(action_id, failing_executor)

        action = self.conn._db['actions'][action_id]
        self.assertEqual(action['status'], 'failed')
        self.assertIn('Permission denied', action['error'])

    def test_execute_only_planned_actions(self):
        action_id = self.am.plan_action('s1', 'file.read', 'Read file')
        # Execute once
        self.am.execute_action(action_id, lambda p: {
            'result': {}, 'result_summary': 'ok', 'snapshots': [],
        })
        # Try to execute again — should fail
        with self.assertRaises(ValueError) as ctx:
            self.am.execute_action(action_id, lambda p: {})
        self.assertIn('completed', str(ctx.exception))

    def test_execute_nonexistent_action(self):
        with self.assertRaises(ValueError) as ctx:
            self.am.execute_action(9999, lambda p: {})
        self.assertIn('not found', str(ctx.exception))

    def test_execute_multiple_snapshots(self):
        action_id = self.am.plan_action('s1', 'config.yaml.edit', 'Update configs')

        def executor(params):
            return {
                'result': {'files_updated': 2},
                'result_summary': 'Updated 2 config files',
                'snapshots': [
                    {
                        'resource_type': 'config',
                        'resource_id': 'configs/wotan.yaml',
                        'content_before': 'port: 18000',
                        'content_after': 'port: 18001',
                    },
                    {
                        'resource_type': 'config',
                        'resource_id': 'configs/sophia.yaml',
                        'content_before': 'port: 19005',
                        'content_after': 'port: 19006',
                    },
                ],
            }

        self.am.execute_action(action_id, executor)
        snapshots = [s for s in self.conn._db['snapshots'] if s['action_id'] == action_id]
        self.assertEqual(len(snapshots), 2)


class TestRevertAction(unittest.TestCase):
    """Test the revert workflow."""

    def setUp(self):
        self.conn = MockConnection()
        self.am = ActionManager(conn=self.conn)

    def _create_and_execute(self):
        """Helper: create and execute a file.write action."""
        action_id = self.am.plan_action(
            session_id='s1',
            action_type='file.write',
            intent='Write config',
            parameters={'path': '/tmp/test.conf'},
            revertable=True,
        )
        self.am.execute_action(action_id, lambda p: {
            'result': {'bytes_written': 100},
            'result_summary': 'Wrote config',
            'snapshots': [{
                'resource_type': 'file',
                'resource_id': '/tmp/test.conf',
                'content_before': 'original content',
                'content_after': 'new content',
            }],
        })
        return action_id

    def test_revert_completed_action(self):
        action_id = self._create_and_execute()

        result = self.am.revert_action(action_id)

        # Original action should be reverted
        original = self.conn._db['actions'][action_id]
        self.assertEqual(original['status'], 'reverted')
        self.assertIsNotNone(original['reverted_by'])

        # Revert action should exist and be completed
        revert_id = result['revert_action_id']
        revert = self.conn._db['actions'][revert_id]
        self.assertEqual(revert['status'], 'completed')
        self.assertEqual(revert['reverts_action'], action_id)
        self.assertIn(f'Revert action {action_id}', revert['intent'])

        # Revert should have created its own snapshots
        revert_snaps = [s for s in self.conn._db['snapshots'] if s['action_id'] == revert_id]
        self.assertEqual(len(revert_snaps), 1)
        self.assertEqual(revert_snaps[0]['content_after'], 'original content')

    def test_revert_already_reverted(self):
        action_id = self._create_and_execute()
        self.am.revert_action(action_id)

        with self.assertRaises(ValueError) as ctx:
            self.am.revert_action(action_id)
        self.assertIn('already reverted', str(ctx.exception))

    def test_revert_planned_action_fails(self):
        action_id = self.am.plan_action('s1', 'file.write', 'Write config')

        with self.assertRaises(ValueError) as ctx:
            self.am.revert_action(action_id)
        self.assertIn('planned', str(ctx.exception))

    def test_revert_no_snapshots_fails(self):
        action_id = self.am.plan_action('s1', 'file.read', 'Read file')
        # Execute with no snapshots
        self.am.execute_action(action_id, lambda p: {
            'result': {}, 'result_summary': 'ok', 'snapshots': [],
        })

        with self.assertRaises(ValueError) as ctx:
            self.am.revert_action(action_id)
        self.assertIn('no snapshots', str(ctx.exception))


class TestSnapshotsAppendOnly(unittest.TestCase):
    """Test that snapshots are append-only (no update, no delete)."""

    def setUp(self):
        self.conn = MockConnection()
        self.am = ActionManager(conn=self.conn)

    def test_snapshots_only_grow(self):
        """Execute multiple actions; snapshot list only grows."""
        initial_count = len(self.conn._db['snapshots'])

        # Action 1
        id1 = self.am.plan_action('s1', 'file.write', 'Write A')
        self.am.execute_action(id1, lambda p: {
            'result': {}, 'result_summary': 'ok',
            'snapshots': [{
                'resource_type': 'file', 'resource_id': '/a',
                'content_before': 'A-old', 'content_after': 'A-new',
            }],
        })
        count_after_1 = len(self.conn._db['snapshots'])
        self.assertGreater(count_after_1, initial_count)

        # Action 2
        id2 = self.am.plan_action('s1', 'file.write', 'Write B')
        self.am.execute_action(id2, lambda p: {
            'result': {}, 'result_summary': 'ok',
            'snapshots': [{
                'resource_type': 'file', 'resource_id': '/b',
                'content_before': 'B-old', 'content_after': 'B-new',
            }],
        })
        count_after_2 = len(self.conn._db['snapshots'])
        self.assertGreater(count_after_2, count_after_1)

        # Revert action 1 — should add MORE snapshots, never remove
        self.am.revert_action(id1)
        count_after_revert = len(self.conn._db['snapshots'])
        self.assertGreater(count_after_revert, count_after_2)

        # All original snapshots still present
        action1_snaps = [s for s in self.conn._db['snapshots'] if s['action_id'] == id1]
        self.assertEqual(len(action1_snaps), 1)
        self.assertEqual(action1_snaps[0]['content_before'], 'A-old')

    def test_snapshot_content_immutable(self):
        """Once a snapshot is written, its content_before never changes."""
        action_id = self.am.plan_action('s1', 'file.write', 'Write')
        self.am.execute_action(action_id, lambda p: {
            'result': {}, 'result_summary': 'ok',
            'snapshots': [{
                'resource_type': 'file', 'resource_id': '/test',
                'content_before': 'sacred-content',
                'content_after': 'new-content',
            }],
        })

        # Verify content
        snap = self.conn._db['snapshots'][0]
        self.assertEqual(snap['content_before'], 'sacred-content')

        # Execute another action on the same resource
        id2 = self.am.plan_action('s1', 'file.write', 'Write again')
        self.am.execute_action(id2, lambda p: {
            'result': {}, 'result_summary': 'ok',
            'snapshots': [{
                'resource_type': 'file', 'resource_id': '/test',
                'content_before': 'new-content',
                'content_after': 'even-newer',
            }],
        })

        # Original snapshot unchanged
        original_snap = self.conn._db['snapshots'][0]
        self.assertEqual(original_snap['content_before'], 'sacred-content')
        self.assertEqual(original_snap['action_id'], action_id)


class TestActionHistory(unittest.TestCase):
    """Test action history querying."""

    def setUp(self):
        self.conn = MockConnection()
        self.am = ActionManager(conn=self.conn)

    def test_history_returns_actions(self):
        self.am.plan_action('s1', 'file.read', 'Read A')
        self.am.plan_action('s1', 'file.read', 'Read B')
        self.am.plan_action('s2', 'file.read', 'Read C')  # different session

        history = self.am.get_action_history('s1')
        self.assertEqual(len(history), 2)
        # All should be from session s1
        for action in history:
            self.assertEqual(action['session_id'], 's1')

    def test_history_respects_limit(self):
        for i in range(10):
            self.am.plan_action('s1', 'file.read', f'Read {i}')

        history = self.am.get_action_history('s1', limit=3)
        self.assertEqual(len(history), 3)

    def test_history_empty_session(self):
        history = self.am.get_action_history('nonexistent')
        self.assertEqual(len(history), 0)


class TestTrinodaNecessitas(unittest.TestCase):
    """Test Trinoda Necessitas: The Well must be alive for writes."""

    def test_no_connection_raises(self):
        """ActionManager with no DB connection refuses write operations."""
        am = ActionManager(conn=None)
        with self.assertRaises(WellUnavailableError) as ctx:
            am.plan_action('s1', 'file.write', 'Write config')
        self.assertIn('not connected', str(ctx.exception))
        self.assertIn('audit trail', str(ctx.exception))

    def test_dead_connection_raises(self):
        """ActionManager with a dead connection refuses operations."""
        dead = DeadConnection()
        # Patch _connect to return None (simulate no DB available)
        am = ActionManager(conn=dead)
        am._connect = lambda: None
        with self.assertRaises(WellUnavailableError) as ctx:
            am.plan_action('s1', 'file.write', 'Write config')
        self.assertIn('unreachable', str(ctx.exception))

    def test_dead_connection_blocks_execute(self):
        """Cannot execute when The Well goes down mid-operation."""
        am = ActionManager(conn=None)
        with self.assertRaises(WellUnavailableError):
            am.execute_action(1, lambda p: {})

    def test_dead_connection_blocks_revert(self):
        """Cannot revert when The Well is down."""
        am = ActionManager(conn=None)
        with self.assertRaises(WellUnavailableError):
            am.revert_action(1)

    def test_dead_connection_blocks_history(self):
        """Cannot query history when The Well is down."""
        am = ActionManager(conn=None)
        with self.assertRaises(WellUnavailableError):
            am.get_action_history('s1')

    def test_error_message_mentions_read_only(self):
        """Error message tells user Zhen enters read-only mode."""
        dead = DeadConnection()
        am = ActionManager(conn=dead)
        am._connect = lambda: None
        with self.assertRaises(WellUnavailableError) as ctx:
            am.plan_action('s1', 'file.write', 'Write')
        self.assertIn('read-only', str(ctx.exception))


class TestGetAction(unittest.TestCase):
    """Test single action retrieval."""

    def setUp(self):
        self.conn = MockConnection()
        self.am = ActionManager(conn=self.conn)

    def test_get_existing_action(self):
        action_id = self.am.plan_action('s1', 'file.read', 'Read file')
        action = self.am.get_action(action_id)
        self.assertIsNotNone(action)
        self.assertEqual(action['id'], action_id)
        self.assertEqual(action['status'], 'planned')

    def test_get_nonexistent_action(self):
        action = self.am.get_action(9999)
        self.assertIsNone(action)


class TestGetSnapshots(unittest.TestCase):
    """Test snapshot retrieval."""

    def setUp(self):
        self.conn = MockConnection()
        self.am = ActionManager(conn=self.conn)

    def test_get_snapshots(self):
        action_id = self.am.plan_action('s1', 'file.write', 'Write')
        self.am.execute_action(action_id, lambda p: {
            'result': {}, 'result_summary': 'ok',
            'snapshots': [{
                'resource_type': 'file', 'resource_id': '/test',
                'content_before': 'old', 'content_after': 'new',
            }],
        })

        snaps = self.am.get_snapshots(action_id)
        self.assertEqual(len(snaps), 1)
        self.assertEqual(snaps[0]['resource_type'], 'file')
        self.assertEqual(snaps[0]['content_before'], 'old')

    def test_get_snapshots_empty(self):
        action_id = self.am.plan_action('s1', 'file.read', 'Read')
        self.am.execute_action(action_id, lambda p: {
            'result': {}, 'result_summary': 'ok', 'snapshots': [],
        })
        snaps = self.am.get_snapshots(action_id)
        self.assertEqual(len(snaps), 0)


class TestFullWorkflow(unittest.TestCase):
    """End-to-end test: plan -> execute -> query -> revert -> verify."""

    def setUp(self):
        self.conn = MockConnection()
        self.am = ActionManager(conn=self.conn)

    def test_full_lifecycle(self):
        # 1. Plan
        action_id = self.am.plan_action(
            session_id='lifecycle-test',
            action_type='file.write',
            intent='Write haproxy config',
            parameters={'path': 'haproxy.cfg', 'content': 'frontend...'},
            revertable=True,
            trace_id='trace-lifecycle',
        )
        action = self.am.get_action(action_id)
        self.assertEqual(action['status'], 'planned')

        # 2. Execute with snapshot
        self.am.execute_action(action_id, lambda p: {
            'result': {'bytes_written': 256},
            'result_summary': 'Wrote haproxy config',
            'snapshots': [{
                'resource_type': 'file',
                'resource_id': 'haproxy.cfg',
                'content_before': '# empty',
                'content_after': 'frontend...',
                'metadata': {'permissions': '0644', 'owner': 'root'},
            }],
        })
        action = self.am.get_action(action_id)
        self.assertEqual(action['status'], 'completed')
        self.assertEqual(action['result']['bytes_written'], 256)

        # 3. Query history
        history = self.am.get_action_history('lifecycle-test')
        self.assertEqual(len(history), 1)
        self.assertEqual(history[0]['action_type'], 'file.write')

        # 4. Verify snapshot
        snaps = self.am.get_snapshots(action_id)
        self.assertEqual(len(snaps), 1)
        self.assertEqual(snaps[0]['content_before'], '# empty')

        # 5. Revert
        revert_result = self.am.revert_action(action_id)
        revert_id = revert_result['revert_action_id']

        # 6. Verify revert
        original = self.am.get_action(action_id)
        self.assertEqual(original['status'], 'reverted')
        self.assertEqual(original['reverted_by'], revert_id)

        revert = self.am.get_action(revert_id)
        self.assertEqual(revert['status'], 'completed')
        self.assertEqual(revert['reverts_action'], action_id)

        # 7. History now shows 2 actions
        history = self.am.get_action_history('lifecycle-test')
        self.assertEqual(len(history), 2)

        # 8. Snapshot count grew (original + revert)
        total_snaps = len(self.conn._db['snapshots'])
        self.assertEqual(total_snaps, 2)  # 1 for write + 1 for revert


if __name__ == '__main__':
    unittest.main()
