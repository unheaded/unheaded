/**
 * app.js - Main Application Logic
 * Orchestrates all modules and handles user interactions
 */

/**
 * Main Application for The Unheaded Kingdom's Kanban Board
 */
const App = (function() {
    'use strict';

    // DOM Element references
    const elements = {
        // Header
        newTaskBtn: null,
        connectionStatus: null,

        // Task Modal (Create/Edit)
        taskModal: null,
        taskForm: null,
        taskId: null,
        taskTitle: null,
        taskDescription: null,
        taskPriority: null,
        taskStatus: null,
        taskAssignee: null,
        taskDueDate: null,
        taskTags: null,
        modalTitle: null,
        modalSubmit: null,
        modalClose: null,
        modalCancel: null,

        // Task Detail Modal
        taskDetailModal: null,
        detailTitle: null,
        detailPriority: null,
        detailDescription: null,
        detailStatus: null,
        detailAssignee: null,
        detailDueDate: null,
        detailCreated: null,
        detailTags: null,
        detailEdit: null,
        detailDelete: null,
        detailClose: null,

        // Delete Confirmation Modal
        deleteModal: null,
        deleteTaskTitle: null,
        deleteConfirm: null,
        deleteCancel: null,
        deleteClose: null
    };

    // Current state
    let currentTask = null;
    let isEditMode = false;

    /**
     * Initialize the application
     */
    async function init() {
        console.log('[App] Initializing The Unheaded Kingdom Kanban Board...');

        // Cache DOM elements
        cacheElements();

        // Initialize modules
        Board.init();

        // Set up event listeners
        setupEventListeners();

        // Connect WebSocket
        setupWebSocket();

        // Load initial data
        await Board.loadTasks();

        console.log('[App] Initialization complete');
    }

    /**
     * Cache DOM element references
     */
    function cacheElements() {
        // Header
        elements.newTaskBtn = document.getElementById('newTaskBtn');
        elements.connectionStatus = document.getElementById('connectionStatus');

        // Task Modal
        elements.taskModal = document.getElementById('taskModal');
        elements.taskForm = document.getElementById('taskForm');
        elements.taskId = document.getElementById('taskId');
        elements.taskTitle = document.getElementById('taskTitle');
        elements.taskDescription = document.getElementById('taskDescription');
        elements.taskPriority = document.getElementById('taskPriority');
        elements.taskStatus = document.getElementById('taskStatus');
        elements.taskAssignee = document.getElementById('taskAssignee');
        elements.taskDueDate = document.getElementById('taskDueDate');
        elements.taskTags = document.getElementById('taskTags');
        elements.modalTitle = document.getElementById('modalTitle');
        elements.modalSubmit = document.getElementById('modalSubmit');
        elements.modalClose = document.getElementById('modalClose');
        elements.modalCancel = document.getElementById('modalCancel');

        // Task Detail Modal
        elements.taskDetailModal = document.getElementById('taskDetailModal');
        elements.detailTitle = document.getElementById('detailTitle');
        elements.detailPriority = document.getElementById('detailPriority');
        elements.detailDescription = document.getElementById('detailDescription');
        elements.detailStatus = document.getElementById('detailStatus');
        elements.detailAssignee = document.getElementById('detailAssignee');
        elements.detailDueDate = document.getElementById('detailDueDate');
        elements.detailCreated = document.getElementById('detailCreated');
        elements.detailTags = document.getElementById('detailTags');
        elements.detailEdit = document.getElementById('detailEdit');
        elements.detailDelete = document.getElementById('detailDelete');
        elements.detailClose = document.getElementById('detailClose');

        // Delete Modal
        elements.deleteModal = document.getElementById('deleteModal');
        elements.deleteTaskTitle = document.getElementById('deleteTaskTitle');
        elements.deleteConfirm = document.getElementById('deleteConfirm');
        elements.deleteCancel = document.getElementById('deleteCancel');
        elements.deleteClose = document.getElementById('deleteClose');
    }

    /**
     * Set up event listeners
     */
    function setupEventListeners() {
        // New Task button
        elements.newTaskBtn?.addEventListener('click', openNewTaskModal);

        // Task Modal events
        elements.taskForm?.addEventListener('submit', handleTaskSubmit);
        elements.modalClose?.addEventListener('click', closeTaskModal);
        elements.modalCancel?.addEventListener('click', closeTaskModal);

        // Task Detail Modal events
        elements.detailClose?.addEventListener('click', closeDetailModal);
        elements.detailEdit?.addEventListener('click', handleDetailEdit);
        elements.detailDelete?.addEventListener('click', handleDetailDelete);

        // Delete Modal events
        elements.deleteClose?.addEventListener('click', closeDeleteModal);
        elements.deleteCancel?.addEventListener('click', closeDeleteModal);
        elements.deleteConfirm?.addEventListener('click', confirmDelete);

        // Modal overlay clicks
        elements.taskModal?.addEventListener('click', (e) => {
            if (e.target === elements.taskModal) closeTaskModal();
        });
        elements.taskDetailModal?.addEventListener('click', (e) => {
            if (e.target === elements.taskDetailModal) closeDetailModal();
        });
        elements.deleteModal?.addEventListener('click', (e) => {
            if (e.target === elements.deleteModal) closeDeleteModal();
        });

        // Keyboard shortcuts
        document.addEventListener('keydown', handleKeyboard);

        // Custom events from Cards module
        window.addEventListener('task:view', (e) => openDetailModal(e.detail.task));
        window.addEventListener('task:edit', (e) => openEditTaskModal(e.detail.task));
        window.addEventListener('task:delete', (e) => openDeleteModal(e.detail.task));
        window.addEventListener('task:move', (e) => handleTaskMove(e.detail));
        window.addEventListener('task:update', (e) => handleInlineUpdate(e.detail));
    }

    /**
     * Set up WebSocket connection
     */
    function setupWebSocket() {
        // Status change handler
        WebSocketClient.onStatusChange((state) => {
            updateConnectionStatus(state);
        });

        // Event handlers
        WebSocketClient.on('tasks.created', Board.handleTaskCreated);
        WebSocketClient.on('tasks.updated', Board.handleTaskUpdated);
        WebSocketClient.on('tasks.completed', Board.handleTaskCompleted);
        WebSocketClient.on('tasks.deleted', Board.handleTaskDeleted);
        WebSocketClient.on('timeline.updates', (data) => {
            console.log('[App] Timeline update:', data);
        });

        // Connect
        WebSocketClient.connect().catch(error => {
            console.warn('[App] WebSocket connection failed:', error);
        });
    }

    /**
     * Update connection status indicator
     * @param {string} state - Connection state
     */
    function updateConnectionStatus(state) {
        if (!elements.connectionStatus) return;

        const statusText = elements.connectionStatus.querySelector('.status-text');

        // Remove all state classes
        elements.connectionStatus.classList.remove('connected', 'disconnected', 'connecting');

        switch (state) {
            case WebSocketClient.ConnectionState.CONNECTED:
                elements.connectionStatus.classList.add('connected');
                statusText.textContent = 'Connected';
                break;
            case WebSocketClient.ConnectionState.CONNECTING:
            case WebSocketClient.ConnectionState.RECONNECTING:
                elements.connectionStatus.classList.add('connecting');
                statusText.textContent = state === WebSocketClient.ConnectionState.RECONNECTING ?
                    'Reconnecting...' : 'Connecting...';
                break;
            default:
                elements.connectionStatus.classList.add('disconnected');
                statusText.textContent = 'Disconnected';
        }
    }

    /**
     * Handle keyboard shortcuts
     * @param {KeyboardEvent} e - Keyboard event
     */
    function handleKeyboard(e) {
        // Escape closes modals
        if (e.key === 'Escape') {
            if (elements.deleteModal?.classList.contains('active')) {
                closeDeleteModal();
            } else if (elements.taskDetailModal?.classList.contains('active')) {
                closeDetailModal();
            } else if (elements.taskModal?.classList.contains('active')) {
                closeTaskModal();
            }
        }

        // Ctrl/Cmd + N for new task
        if ((e.ctrlKey || e.metaKey) && e.key === 'n') {
            e.preventDefault();
            openNewTaskModal();
        }
    }

    /* ============================================
       Task Modal (Create/Edit)
       ============================================ */

    /**
     * Open modal for creating a new task
     */
    function openNewTaskModal() {
        isEditMode = false;
        currentTask = null;

        // Reset form
        elements.taskForm.reset();
        elements.taskId.value = '';

        // Set defaults
        elements.taskPriority.value = '2';
        elements.taskStatus.value = 'backlog';

        // Update modal title and button
        elements.modalTitle.textContent = 'New Task';
        elements.modalSubmit.textContent = 'Create Task';

        // Show modal
        elements.taskModal.classList.add('active');
        elements.taskTitle.focus();
    }

    /**
     * Open modal for editing a task
     * @param {Object} task - Task to edit
     */
    function openEditTaskModal(task) {
        isEditMode = true;
        currentTask = task;

        // Close detail modal if open
        closeDetailModal();

        // Populate form
        elements.taskId.value = task.id;
        elements.taskTitle.value = task.title;
        elements.taskDescription.value = task.description || '';
        elements.taskPriority.value = task.priority;
        elements.taskStatus.value = task.status;
        elements.taskAssignee.value = task.assignee || '';
        elements.taskDueDate.value = task.due_date ? task.due_date.split('T')[0] : '';
        elements.taskTags.value = task.tags ? task.tags.join(', ') : '';

        // Update modal title and button
        elements.modalTitle.textContent = 'Edit Task';
        elements.modalSubmit.textContent = 'Save Changes';

        // Show modal
        elements.taskModal.classList.add('active');
        elements.taskTitle.focus();
    }

    /**
     * Close task modal
     */
    function closeTaskModal() {
        elements.taskModal.classList.remove('active');
        elements.taskForm.reset();
        currentTask = null;
        isEditMode = false;
    }

    /**
     * Handle task form submission
     * @param {Event} e - Submit event
     */
    async function handleTaskSubmit(e) {
        e.preventDefault();

        const taskData = {
            title: elements.taskTitle.value.trim(),
            description: elements.taskDescription.value.trim(),
            priority: parseInt(elements.taskPriority.value, 10),
            status: elements.taskStatus.value,
            assignee: elements.taskAssignee.value.trim(),
            due_date: elements.taskDueDate.value || null,
            tags: elements.taskTags.value
        };

        // Validate
        if (!taskData.title) {
            Board.showToast('error', 'Title required', 'Please enter a task title');
            elements.taskTitle.focus();
            return;
        }

        // Disable form during submission
        elements.modalSubmit.disabled = true;
        elements.modalSubmit.textContent = isEditMode ? 'Saving...' : 'Creating...';

        try {
            let task;

            if (isEditMode && currentTask) {
                // Update existing task
                task = await API.tasks.update(currentTask.id, taskData);
                Board.updateTask(task);
                Board.showToast('success', 'Task updated', task.title);
            } else {
                // Create new task
                task = await API.tasks.create(taskData);
                Board.addTask(task);
                Board.showToast('success', 'Task created', task.title);
            }

            closeTaskModal();

        } catch (error) {
            console.error('[App] Task submission failed:', error);
            Board.showToast('error', 'Operation failed', error.message);
        } finally {
            elements.modalSubmit.disabled = false;
            elements.modalSubmit.textContent = isEditMode ? 'Save Changes' : 'Create Task';
        }
    }

    /* ============================================
       Task Detail Modal
       ============================================ */

    /**
     * Open task detail modal
     * @param {Object} task - Task to display
     */
    function openDetailModal(task) {
        currentTask = task;

        const priority = Cards.PRIORITIES[task.priority] || Cards.PRIORITIES[2];

        // Update content
        elements.detailTitle.textContent = task.title;
        elements.detailPriority.textContent = priority.label;
        elements.detailPriority.className = `priority-badge ${priority.class}`;
        elements.detailDescription.textContent = task.description || 'No description provided.';
        elements.detailStatus.textContent = Cards.STATUS_NAMES[task.status] || task.status;
        elements.detailAssignee.textContent = task.assignee || 'Unassigned';
        elements.detailDueDate.textContent = task.due_date ? Cards.formatDate(task.due_date) : 'Not set';
        elements.detailCreated.textContent = task.created_at ?
            new Date(task.created_at).toLocaleDateString('en-US', {
                year: 'numeric',
                month: 'short',
                day: 'numeric',
                hour: '2-digit',
                minute: '2-digit'
            }) : '-';

        // Update tags
        if (task.tags && task.tags.length > 0) {
            elements.detailTags.innerHTML = task.tags
                .map(tag => `<span class="tag">${Cards.escapeHtml(tag)}</span>`)
                .join('');
        } else {
            elements.detailTags.innerHTML = '<span class="text-muted">No tags</span>';
        }

        // Show modal
        elements.taskDetailModal.classList.add('active');
    }

    /**
     * Close task detail modal
     */
    function closeDetailModal() {
        elements.taskDetailModal.classList.remove('active');
    }

    /**
     * Handle edit button in detail modal
     */
    function handleDetailEdit() {
        if (currentTask) {
            openEditTaskModal(currentTask);
        }
    }

    /**
     * Handle delete button in detail modal
     */
    function handleDetailDelete() {
        if (currentTask) {
            closeDetailModal();
            openDeleteModal(currentTask);
        }
    }

    /* ============================================
       Delete Confirmation Modal
       ============================================ */

    /**
     * Open delete confirmation modal
     * @param {Object} task - Task to delete
     */
    function openDeleteModal(task) {
        currentTask = task;
        elements.deleteTaskTitle.textContent = task.title;
        elements.deleteModal.classList.add('active');
    }

    /**
     * Close delete modal
     */
    function closeDeleteModal() {
        elements.deleteModal.classList.remove('active');
    }

    /**
     * Confirm and execute delete
     */
    async function confirmDelete() {
        if (!currentTask) return;

        const taskId = currentTask.id;
        const taskTitle = currentTask.title;

        elements.deleteConfirm.disabled = true;
        elements.deleteConfirm.textContent = 'Deleting...';

        try {
            await API.tasks.delete(taskId);
            Board.removeTask(taskId);
            Board.showToast('success', 'Task deleted', taskTitle);
            closeDeleteModal();

        } catch (error) {
            console.error('[App] Delete failed:', error);
            Board.showToast('error', 'Delete failed', error.message);
        } finally {
            elements.deleteConfirm.disabled = false;
            elements.deleteConfirm.textContent = 'Delete Task';
            currentTask = null;
        }
    }

    /* ============================================
       Drag and Drop Handling
       ============================================ */

    /**
     * Handle task move from drag and drop
     * @param {Object} detail - Move details (taskId, newStatus, cardElement)
     */
    async function handleTaskMove(detail) {
        const { taskId, newStatus, cardElement } = detail;

        const task = Board.getTask(taskId);
        if (!task || task.status === newStatus) return;

        await Board.moveTask(taskId, newStatus);
    }

    /**
     * Handle inline task update
     * @param {Object} detail - Update details (taskId, updates, cardElement)
     */
    async function handleInlineUpdate(detail) {
        const { taskId, updates, cardElement } = detail;

        try {
            const updatedTask = await API.tasks.update(taskId, updates);
            Board.updateTask(updatedTask);
            Board.showToast('success', 'Task updated');
        } catch (error) {
            console.error('[App] Inline update failed:', error);
            Board.showToast('error', 'Update failed', error.message);
            // Refresh the column to restore original state
            const task = Board.getTask(taskId);
            if (task) {
                Board.renderColumn(task.status);
            }
        }
    }

    /* ============================================
       Public API
       ============================================ */

    return {
        init,
        openNewTaskModal,
        openEditTaskModal,
        openDetailModal,
        openDeleteModal
    };
})();

// Initialize when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    App.init().catch(error => {
        console.error('[App] Initialization failed:', error);
    });
});

// Make available globally for debugging
window.App = App;
