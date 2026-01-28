// Board visualization and card rendering
// Handles task display and updates

(function() {
    'use strict';

    // State
    let tasks = [];
    let connected = false;

    // DOM elements
    const columns = {
        todo: document.querySelector('#todo .cards'),
        'in-progress': document.querySelector('#in-progress .cards'),
        done: document.querySelector('#done .cards')
    };

    // Initialize board
    function init() {
        createStatsBar();
        createConnectionStatus();
        createToastContainer();
        showLoading();
    }

    // Create stats bar
    function createStatsBar() {
        const main = document.querySelector('main');
        const board = document.getElementById('kanban-board');

        const statsBar = document.createElement('div');
        statsBar.className = 'stats-bar';
        statsBar.innerHTML = `
            <div class="stat">
                <span class="stat-value" id="stat-total">0</span>
                <span class="stat-label">Total Tasks</span>
            </div>
            <div class="stat">
                <span class="stat-value" id="stat-progress">0%</span>
                <span class="stat-label">Progress</span>
            </div>
            <div class="stat">
                <span class="stat-value" id="stat-velocity">-</span>
                <span class="stat-label">Velocity</span>
            </div>
            <div class="stat">
                <span class="stat-value" id="stat-eta">-</span>
                <span class="stat-label">Alpha ETA</span>
            </div>
        `;

        main.insertBefore(statsBar, board);
    }

    // Create connection status indicator
    function createConnectionStatus() {
        const status = document.createElement('div');
        status.className = 'connection-status connecting';
        status.id = 'connection-status';
        status.innerHTML = `
            <span class="connection-dot"></span>
            <span class="connection-text">Connecting...</span>
        `;
        document.body.appendChild(status);
    }

    // Create toast container
    function createToastContainer() {
        const container = document.createElement('div');
        container.className = 'toast-container';
        container.id = 'toast-container';
        document.body.appendChild(container);
    }

    // Show loading state
    function showLoading() {
        Object.values(columns).forEach(column => {
            column.innerHTML = `
                <div class="loading">
                    <div class="spinner"></div>
                    <span>Loading...</span>
                </div>
            `;
        });
    }

    // Show empty state
    function showEmpty(column) {
        column.innerHTML = `
            <div class="empty-state">
                <div class="empty-state-icon">📭</div>
                <span>No tasks</span>
            </div>
        `;
    }

    // Update connection status
    function updateConnectionStatus(status, text) {
        const el = document.getElementById('connection-status');
        if (!el) return;

        el.className = `connection-status ${status}`;
        el.querySelector('.connection-text').textContent = text;
        connected = status === 'connected';
    }

    // Show toast notification
    function showToast(message, type = 'info') {
        const container = document.getElementById('toast-container');
        if (!container) return;

        const toast = document.createElement('div');
        toast.className = `toast ${type}`;
        toast.textContent = message;
        container.appendChild(toast);

        setTimeout(() => {
            toast.style.animation = 'slideIn 0.3s ease reverse';
            setTimeout(() => toast.remove(), 300);
        }, 3000);
    }

    // Create card element
    function createCard(task) {
        const card = document.createElement('div');
        card.className = 'card';
        card.dataset.id = task.id;

        const typeClass = (task.type || 'task').toLowerCase();
        const progress = task.progress || 0;

        card.innerHTML = `
            <div class="card-title">${escapeHtml(task.title)}</div>
            ${task.description ? `<div class="card-description">${escapeHtml(task.description)}</div>` : ''}
            <div class="card-meta">
                <span class="card-type ${typeClass}">${task.type || 'Task'}</span>
                <span class="card-owner">${escapeHtml(task.owner || 'Unassigned')}</span>
            </div>
            ${progress > 0 ? `
            <div class="progress-bar">
                <div class="fill" style="width: ${progress}%"></div>
            </div>
            ` : ''}
        `;

        card.addEventListener('click', () => {
            console.log('Card clicked:', task);
            // TODO: Open task detail modal
        });

        return card;
    }

    // Escape HTML to prevent XSS
    function escapeHtml(text) {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // Render tasks to board
    function render(taskList) {
        tasks = taskList;

        // Group by status
        const grouped = {
            todo: [],
            'in-progress': [],
            done: []
        };

        tasks.forEach(task => {
            const status = normalizeStatus(task.status);
            if (grouped[status]) {
                grouped[status].push(task);
            }
        });

        // Render each column
        Object.entries(columns).forEach(([status, column]) => {
            const columnTasks = grouped[status] || [];

            if (columnTasks.length === 0) {
                showEmpty(column);
            } else {
                column.innerHTML = '';
                columnTasks.forEach(task => {
                    column.appendChild(createCard(task));
                });
            }

            // Update count in header
            const header = column.parentElement.querySelector('h2');
            let countSpan = header.querySelector('.count');
            if (!countSpan) {
                countSpan = document.createElement('span');
                countSpan.className = 'count';
                header.appendChild(countSpan);
            }
            countSpan.textContent = columnTasks.length;
        });

        // Update stats
        updateStats();
    }

    // Normalize status string
    function normalizeStatus(status) {
        if (!status) return 'todo';
        const s = status.toLowerCase().replace(/[_\s]+/g, '-');
        if (s.includes('progress') || s === 'in_progress' || s === 'in-progress') return 'in-progress';
        if (s.includes('done') || s.includes('complete') || s === 'completed') return 'done';
        return 'todo';
    }

    // Update stats bar
    function updateStats() {
        const total = tasks.length;
        const done = tasks.filter(t => normalizeStatus(t.status) === 'done').length;
        const progress = total > 0 ? Math.round((done / total) * 100) : 0;

        document.getElementById('stat-total').textContent = total;
        document.getElementById('stat-progress').textContent = `${progress}%`;

        // TODO: Calculate velocity from historical data
        // TODO: Calculate ETA from timeline
    }

    // Update single card (for live updates)
    function updateCard(task) {
        const existing = document.querySelector(`.card[data-id="${task.id}"]`);
        const newCard = createCard(task);
        const targetColumn = columns[normalizeStatus(task.status)];

        if (existing) {
            // Check if status changed (card needs to move)
            const currentColumn = existing.parentElement;
            if (currentColumn !== targetColumn) {
                existing.remove();
                targetColumn.appendChild(newCard);
                showToast(`Task moved: ${task.title}`, 'info');
            } else {
                existing.replaceWith(newCard);
            }
        } else {
            // New task
            targetColumn.appendChild(newCard);
            showToast(`New task: ${task.title}`, 'success');
        }

        // Update tasks array
        const idx = tasks.findIndex(t => t.id === task.id);
        if (idx >= 0) {
            tasks[idx] = task;
        } else {
            tasks.push(task);
        }

        updateStats();
    }

    // Remove card
    function removeCard(taskId) {
        const card = document.querySelector(`.card[data-id="${taskId}"]`);
        if (card) {
            card.style.animation = 'slideIn 0.3s ease reverse';
            setTimeout(() => {
                card.remove();
                tasks = tasks.filter(t => t.id !== taskId);
                updateStats();
            }, 300);
        }
    }

    // Export for timeline-reader
    window.KanbanBoard = {
        init,
        render,
        updateCard,
        removeCard,
        updateConnectionStatus,
        showToast,
        getTasks: () => tasks,
        isConnected: () => connected
    };

    // Auto-init when DOM ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    console.log('board-viz.js loaded');
})();
