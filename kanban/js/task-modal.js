// Task Detail Modal
// Handles opening, editing, saving, and deleting tasks via modal dialog

(function() {
    'use strict';

    // API config (matches timeline-reader)
    const API_BASE = '/api/v1';

    // State
    let currentTask = null;
    let isEditMode = false;
    let isDeleting = false;

    // DOM references (cached on first use)
    let overlay, container, closeBtn;
    let viewMode, editMode;
    let viewActions, editActions;
    let deleteConfirm;

    // View mode elements
    let elTitle, elDescription, elStatus, elPriority, elType, elOwner;
    let elProgressField, elProgressFill, elProgressText;

    // Edit mode elements
    let editTitle, editDescription, editStatus, editPriority, editType, editOwner;

    // Buttons
    let editBtn, deleteBtn, saveBtn, cancelBtn, confirmDeleteBtn, cancelDeleteBtn;

    // Initialize DOM references
    function cacheDom() {
        overlay = document.getElementById('task-modal-overlay');
        container = overlay.querySelector('.modal-container');
        closeBtn = document.getElementById('modal-close-btn');

        viewMode = document.getElementById('modal-view-mode');
        editMode = document.getElementById('modal-edit-mode');
        viewActions = document.getElementById('modal-view-actions');
        editActions = document.getElementById('modal-edit-actions');
        deleteConfirm = document.getElementById('modal-delete-confirm');

        // View elements
        elTitle = document.getElementById('modal-title');
        elDescription = document.getElementById('modal-description');
        elStatus = document.getElementById('modal-status');
        elPriority = document.getElementById('modal-priority');
        elType = document.getElementById('modal-type');
        elOwner = document.getElementById('modal-owner');
        elProgressField = document.getElementById('modal-progress-field');
        elProgressFill = document.getElementById('modal-progress-fill');
        elProgressText = document.getElementById('modal-progress-text');

        // Edit elements
        editTitle = document.getElementById('edit-title');
        editDescription = document.getElementById('edit-description');
        editStatus = document.getElementById('edit-status');
        editPriority = document.getElementById('edit-priority');
        editType = document.getElementById('edit-type');
        editOwner = document.getElementById('edit-owner');

        // Buttons
        editBtn = document.getElementById('modal-edit-btn');
        deleteBtn = document.getElementById('modal-delete-btn');
        saveBtn = document.getElementById('modal-save-btn');
        cancelBtn = document.getElementById('modal-cancel-btn');
        confirmDeleteBtn = document.getElementById('modal-confirm-delete-btn');
        cancelDeleteBtn = document.getElementById('modal-cancel-delete-btn');
    }

    // Bind all event listeners
    function bindEvents() {
        // Close modal
        closeBtn.addEventListener('click', closeModal);
        overlay.addEventListener('click', function(e) {
            if (e.target === overlay) {
                closeModal();
            }
        });

        // Keyboard
        document.addEventListener('keydown', function(e) {
            if (!overlay.classList.contains('active')) return;

            if (e.key === 'Escape') {
                if (isDeleting) {
                    hideDeleteConfirm();
                } else if (isEditMode) {
                    switchToViewMode();
                } else {
                    closeModal();
                }
            }
        });

        // View mode buttons
        editBtn.addEventListener('click', switchToEditMode);
        deleteBtn.addEventListener('click', showDeleteConfirm);

        // Edit mode buttons
        saveBtn.addEventListener('click', saveTask);
        cancelBtn.addEventListener('click', switchToViewMode);

        // Delete confirmation buttons
        confirmDeleteBtn.addEventListener('click', deleteTask);
        cancelDeleteBtn.addEventListener('click', hideDeleteConfirm);
    }

    // ============================
    // Open / Close
    // ============================

    function openModal(task) {
        if (!task) return;
        currentTask = task;
        isEditMode = false;
        isDeleting = false;

        populateViewMode(task);
        showViewMode();
        hideDeleteConfirm();

        overlay.classList.add('active');
        overlay.setAttribute('aria-hidden', 'false');
        document.body.style.overflow = 'hidden';

        // Focus the close button for accessibility
        setTimeout(function() { closeBtn.focus(); }, 100);
    }

    function closeModal() {
        overlay.classList.remove('active');
        overlay.setAttribute('aria-hidden', 'true');
        document.body.style.overflow = '';
        currentTask = null;
        isEditMode = false;
        isDeleting = false;
    }

    // ============================
    // View Mode
    // ============================

    function populateViewMode(task) {
        elTitle.textContent = task.title || 'Untitled Task';
        elDescription.textContent = task.description || 'No description provided.';

        // Status badge
        var statusNorm = normalizeStatus(task.status);
        elStatus.textContent = formatStatus(statusNorm);
        elStatus.className = 'modal-badge status-' + statusNorm;

        // Priority badge
        var priority = (task.priority || 'medium').toLowerCase();
        elPriority.textContent = capitalize(priority);
        elPriority.className = 'modal-badge priority-' + priority;

        // Type badge
        var type = (task.type || 'task').toLowerCase();
        elType.textContent = capitalize(type);
        elType.className = 'modal-badge type-' + type;

        // Owner
        elOwner.textContent = task.owner || 'Unassigned';

        // Progress
        var progress = task.progress || 0;
        if (progress > 0) {
            elProgressField.style.display = '';
            elProgressFill.style.width = progress + '%';
            elProgressText.textContent = progress + '%';
        } else {
            elProgressField.style.display = 'none';
        }
    }

    function showViewMode() {
        viewMode.style.display = '';
        editMode.style.display = 'none';
        viewActions.style.display = '';
        editActions.style.display = 'none';
        isEditMode = false;
    }

    // ============================
    // Edit Mode
    // ============================

    function switchToEditMode() {
        if (!currentTask) return;

        // Populate edit fields from current task
        editTitle.value = currentTask.title || '';
        editDescription.value = currentTask.description || '';
        editStatus.value = normalizeStatus(currentTask.status);
        editPriority.value = (currentTask.priority || 'medium').toLowerCase();
        editType.value = (currentTask.type || 'task').toLowerCase();
        editOwner.value = currentTask.owner || '';

        viewMode.style.display = 'none';
        editMode.style.display = '';
        viewActions.style.display = 'none';
        editActions.style.display = '';
        isEditMode = true;

        // Focus first input
        setTimeout(function() { editTitle.focus(); }, 50);
    }

    function switchToViewMode() {
        if (currentTask) {
            populateViewMode(currentTask);
        }
        showViewMode();
    }

    // ============================
    // Save Task
    // ============================

    async function saveTask() {
        if (!currentTask) return;

        var updatedData = {
            title: editTitle.value.trim(),
            description: editDescription.value.trim(),
            status: editStatus.value,
            priority: editPriority.value,
            type: editType.value,
            owner: editOwner.value.trim()
        };

        // Basic validation
        if (!updatedData.title) {
            editTitle.focus();
            editTitle.style.borderColor = 'var(--accent-danger)';
            setTimeout(function() {
                editTitle.style.borderColor = '';
            }, 2000);
            return;
        }

        // Disable save button while saving
        saveBtn.disabled = true;
        saveBtn.textContent = 'Saving...';

        try {
            var response = await fetch(API_BASE + '/tasks/' + encodeURIComponent(currentTask.id), {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(updatedData)
            });

            if (!response.ok) {
                throw new Error('HTTP ' + response.status);
            }

            var saved = await response.json();

            // Merge updated fields back onto currentTask
            Object.assign(currentTask, updatedData);
            if (saved.task) {
                Object.assign(currentTask, saved.task);
            }

            // Update the board card
            if (window.KanbanBoard && window.KanbanBoard.updateCard) {
                window.KanbanBoard.updateCard(currentTask);
            }

            // Switch back to view mode with updated data
            populateViewMode(currentTask);
            showViewMode();

            if (window.KanbanBoard && window.KanbanBoard.showToast) {
                window.KanbanBoard.showToast('Task updated successfully', 'success');
            }

        } catch (err) {
            console.error('Failed to save task:', err);

            // User-friendly error message
            var friendlyMsg = getFriendlyErrorMessage(err);

            // Optimistic local update on network failure
            Object.assign(currentTask, updatedData);
            if (window.KanbanBoard && window.KanbanBoard.updateCard) {
                window.KanbanBoard.updateCard(currentTask);
            }

            populateViewMode(currentTask);
            showViewMode();

            if (window.KanbanBoard && window.KanbanBoard.showToast) {
                window.KanbanBoard.showToast('Saved locally. ' + friendlyMsg, 'warning');
            }
        } finally {
            saveBtn.disabled = false;
            saveBtn.textContent = 'Save';
        }
    }

    // ============================
    // Delete Task
    // ============================

    function showDeleteConfirm() {
        if (!currentTask) return;
        isDeleting = true;
        var nameEl = deleteConfirm.querySelector('.modal-delete-task-name');
        if (nameEl) {
            nameEl.textContent = currentTask.title || 'Untitled Task';
        }
        deleteConfirm.style.display = '';
    }

    function hideDeleteConfirm() {
        isDeleting = false;
        deleteConfirm.style.display = 'none';
    }

    async function deleteTask() {
        if (!currentTask) return;

        confirmDeleteBtn.disabled = true;
        confirmDeleteBtn.textContent = 'Deleting...';

        try {
            var response = await fetch(API_BASE + '/tasks/' + encodeURIComponent(currentTask.id), {
                method: 'DELETE'
            });

            if (!response.ok) {
                throw new Error('HTTP ' + response.status);
            }

            // Remove from board
            if (window.KanbanBoard && window.KanbanBoard.removeCard) {
                window.KanbanBoard.removeCard(currentTask.id);
            }

            if (window.KanbanBoard && window.KanbanBoard.showToast) {
                window.KanbanBoard.showToast('Task deleted', 'success');
            }

            closeModal();

        } catch (err) {
            console.error('Failed to delete task:', err);

            // Remove locally on network failure
            if (window.KanbanBoard && window.KanbanBoard.removeCard) {
                window.KanbanBoard.removeCard(currentTask.id);
            }

            if (window.KanbanBoard && window.KanbanBoard.showToast) {
                window.KanbanBoard.showToast('Deleted locally (API unavailable)', 'info');
            }

            closeModal();

        } finally {
            confirmDeleteBtn.disabled = false;
            confirmDeleteBtn.textContent = 'Yes, Delete';
        }
    }

    // ============================
    // Helpers
    // ============================

    function normalizeStatus(status) {
        if (!status) return 'todo';
        var s = status.toLowerCase().replace(/[_\s]+/g, '-');
        if (s.includes('progress') || s === 'in-progress') return 'in-progress';
        if (s.includes('done') || s.includes('complete') || s === 'completed') return 'done';
        return 'todo';
    }

    function formatStatus(normalized) {
        switch (normalized) {
            case 'in-progress': return 'In Progress';
            case 'done': return 'Done';
            default: return 'Todo';
        }
    }

    function capitalize(str) {
        if (!str) return '';
        return str.charAt(0).toUpperCase() + str.slice(1);
    }

    // ============================
    // Init
    // ============================

    function init() {
        cacheDom();
        if (!overlay) {
            console.warn('task-modal.js: modal overlay not found in DOM');
            return;
        }
        bindEvents();
        console.log('task-modal.js loaded');
    }

    // Export
    window.TaskModal = {
        open: openModal,
        close: closeModal
    };

    // Auto-init
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
