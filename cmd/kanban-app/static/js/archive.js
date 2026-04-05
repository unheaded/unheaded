/**
 * Archive/Deleted/Completed viewer for Kanban board.
 * All styles via CSS classes (CSP-compliant, no inline styles).
 */
(function() {
    'use strict';

    document.addEventListener('DOMContentLoaded', function() {
        // Archive toggle button
        const archBtn = document.getElementById('archiveViewBtn');
        const archView = document.getElementById('archiveView');
        if (archBtn && archView) {
            archBtn.addEventListener('click', function() {
                const visible = archView.classList.toggle('visible');
                archBtn.textContent = visible ? 'Hide Archive' : 'Archive';
                if (visible) loadArchive('deleted');
            });
        }

        // Tab buttons
        const tabs = {
            'archTabDeleted': 'deleted',
            'archTabArchived': 'archived',
            'archTabCompleted': 'done'
        };
        Object.entries(tabs).forEach(function(entry) {
            var id = entry[0], type = entry[1];
            var btn = document.getElementById(id);
            if (btn) btn.addEventListener('click', function() { loadArchive(type); });
        });

        // GUID click-to-select in detail modal
        var detailGuid = document.getElementById('detailGuid');
        if (detailGuid) {
            detailGuid.addEventListener('click', function() { this.select(); });
        }

        // GUID click-to-select in edit modal
        var editGuid = document.getElementById('taskGuid');
        if (editGuid) {
            editGuid.addEventListener('click', function() { this.select(); });
        }
    });

    async function loadArchive(type) {
        var list = document.getElementById('archiveList');
        if (!list) return;
        list.innerHTML = '';
        var loading = document.createElement('div');
        loading.className = 'archive-empty';
        loading.textContent = 'Loading...';
        list.appendChild(loading);

        try {
            var url;
            if (type === 'deleted') url = '/api/v1/tasks/deleted';
            else if (type === 'archived') url = '/api/v1/tasks/archived';
            else url = '/api/v1/tasks/completed';

            var resp = await fetch(url);
            var data = await resp.json();
            var tasks = data.tasks || [];

            list.innerHTML = '';

            if (tasks.length === 0) {
                var empty = document.createElement('div');
                empty.className = 'archive-empty';
                empty.textContent = 'No ' + type + ' tasks found.';
                list.appendChild(empty);
                return;
            }

            tasks.forEach(function(t) {
                var item = document.createElement('div');
                item.className = 'archive-item';

                var info = document.createElement('div');

                var title = document.createElement('span');
                title.className = 'archive-item-title';
                title.textContent = t.title;
                title.addEventListener('click', function() {
                    if (window.App) App.openDetailModal(t);
                });
                info.appendChild(title);

                var typeSpan = document.createElement('span');
                typeSpan.className = 'archive-item-type';
                typeSpan.textContent = t.type || 'task';
                info.appendChild(typeSpan);

                if (t.guid) {
                    var guid = document.createElement('code');
                    guid.className = 'archive-item-guid';
                    guid.textContent = t.guid.slice(0, 8) + '...';
                    guid.title = t.guid + ' (click to copy)';
                    guid.addEventListener('click', function() {
                        navigator.clipboard.writeText(t.guid);
                    });
                    info.appendChild(guid);
                }

                var actions = document.createElement('div');
                actions.className = 'archive-item-actions';

                var date = t.deleted_at || t.archived_at || t.updated_at || '';
                if (date) {
                    var dateSpan = document.createElement('span');
                    dateSpan.className = 'archive-item-date';
                    dateSpan.textContent = new Date(date).toLocaleDateString();
                    actions.appendChild(dateSpan);
                }

                if (type === 'deleted' || type === 'archived') {
                    var restoreBtn = document.createElement('button');
                    restoreBtn.className = 'archive-restore-btn';
                    restoreBtn.textContent = 'Restore';
                    restoreBtn.addEventListener('click', function() { restoreTask(t.id); });
                    actions.appendChild(restoreBtn);
                }

                item.appendChild(info);
                item.appendChild(actions);
                list.appendChild(item);
            });
        } catch (e) {
            list.innerHTML = '';
            var err = document.createElement('div');
            err.className = 'archive-error';
            err.textContent = 'Error: ' + e.message;
            list.appendChild(err);
        }
    }

    async function restoreTask(id) {
        try {
            await fetch('/api/v1/tasks/' + id + '/restore', { method: 'POST' });
            if (window.Board) Board.showToast('success', 'Task restored', id);
            loadArchive('deleted');
        } catch (e) {
            if (window.Board) Board.showToast('error', 'Restore failed', e.message);
        }
    }

    // Export for global access
    window.loadArchive = loadArchive;
    window.restoreTask = restoreTask;
})();
