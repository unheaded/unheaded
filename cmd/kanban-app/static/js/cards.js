/**
 * cards.js - Card Components and Drag-Drop Functionality
 * Handles task card rendering, interactions, and drag-drop logic
 */

/**
 * Cards module for The Unheaded Kingdom's Kanban Board
 */
const Cards = (function() {
    'use strict';

    // Priority configuration - based on progress percentage
    const PRIORITIES = {
        0: { label: 'P0', name: 'Critical', class: 'p0' },
        1: { label: 'P1', name: 'High', class: 'p1' },
        2: { label: 'P2', name: 'Medium', class: 'p2' },
        3: { label: 'P3', name: 'Low', class: 'p3' },
        4: { label: 'P4', name: 'Backlog', class: 'p4' }
    };

    // Task type to priority mapping
    const TYPE_PRIORITIES = {
        'milestone': 0,
        'bug': 0,
        'infra': 1,
        'feature': 1,
        'task': 2,
        'tech-debt': 3,
        'vision': 3,
        'wishlist': 4,
        'epic': 0,
        'default': 2
    };

    // Status display names - maps backend status to display
    const STATUS_NAMES = {
        'backlog': 'Backlog',
        'todo': 'Backlog',
        'in_progress': 'In Progress',
        'in-progress': 'In Progress',
        'review': 'Review',
        'done': 'Done'
    };

    // Status normalization - maps backend status to frontend status
    const STATUS_MAP = {
        'todo': 'backlog',
        'backlog': 'backlog',
        'in-progress': 'in_progress',
        'in_progress': 'in_progress',
        'review': 'review',
        'done': 'done'
    };

    // Reverse status map for API calls
    const STATUS_TO_API = {
        'backlog': 'todo',
        'in_progress': 'in-progress',
        'review': 'review',
        'done': 'done'
    };

    // Drag state
    let draggedCard = null;
    let draggedTaskId = null;
    let dropPlaceholder = null;
    let isDragging = false;

    // Edit state
    let editingCardId = null;

    /**
     * Format date for display
     * @param {string} dateStr - ISO date string
     * @returns {string}
     */
    function formatDate(dateStr) {
        if (!dateStr) return '';

        const date = new Date(dateStr);
        const now = new Date();
        const diffDays = Math.ceil((date - now) / (1000 * 60 * 60 * 24));

        // Format options
        const options = { month: 'short', day: 'numeric' };

        // Add year if not current year
        if (date.getFullYear() !== now.getFullYear()) {
            options.year = 'numeric';
        }

        return date.toLocaleDateString('en-US', options);
    }

    /**
     * Get due date CSS class based on proximity
     * @param {string} dateStr - ISO date string
     * @returns {string}
     */
    function getDueDateClass(dateStr) {
        if (!dateStr) return '';

        const date = new Date(dateStr);
        const now = new Date();
        now.setHours(0, 0, 0, 0);

        const diffDays = Math.ceil((date - now) / (1000 * 60 * 60 * 24));

        if (diffDays < 0) return 'overdue';
        if (diffDays <= 2) return 'due-soon';
        return '';
    }

    /**
     * Get initials from name
     * @param {string} name - Full name
     * @returns {string}
     */
    function getInitials(name) {
        if (!name) return '?';
        return name
            .split(' ')
            .map(part => part.charAt(0).toUpperCase())
            .slice(0, 2)
            .join('');
    }

    /**
     * Escape HTML to prevent XSS
     * @param {string} text - Text to escape
     * @returns {string}
     */
    function escapeHtml(text) {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    /**
     * Normalize task data from backend to frontend format
     * @param {Object} task - Task from backend
     * @returns {Object} - Normalized task
     */
    function normalizeTask(task) {
        const normalized = { ...task };

        // Normalize status
        normalized.status = STATUS_MAP[task.status] || task.status;

        // Calculate priority from type if not set
        if (normalized.priority === undefined || normalized.priority === null) {
            normalized.priority = TYPE_PRIORITIES[task.type] || TYPE_PRIORITIES['default'];
        }

        // Map owner to assignee
        if (!normalized.assignee && task.owner) {
            normalized.assignee = task.owner;
        }

        return normalized;
    }

    /**
     * Create a task card element
     * @param {Object} task - Task data
     * @returns {HTMLElement}
     */
    function createCard(rawTask) {
        const task = normalizeTask(rawTask);
        const priority = PRIORITIES[task.priority] || PRIORITIES[2];
        const dueDateClass = getDueDateClass(task.due_date);
        const isCompleted = task.status === 'done';

        const card = document.createElement('div');
        card.className = `task-card${isCompleted ? ' completed' : ''}`;
        card.dataset.taskId = task.id;
        card.dataset.priority = task.priority;
        card.dataset.originalStatus = rawTask.status; // Store original status for API calls
        card.draggable = true;
        card.tabIndex = 0;

        // Build progress bar HTML if progress is available
        // Note: progress-fill width is set via CSSOM (element.style) after
        // innerHTML assignment so that CSP style-src does not need 'unsafe-inline'.
        let progressHtml = '';
        if (task.progress !== undefined && task.progress > 0) {
            progressHtml = `
                <div class="card-progress">
                    <div class="progress-bar">
                        <div class="progress-fill" data-progress="${task.progress}"></div>
                    </div>
                    <span class="progress-text">${task.progress}%</span>
                </div>
            `;
        }

        // Build type badge HTML if type is available
        let typeBadge = '';
        if (task.type) {
            typeBadge = `<span class="type-badge type-${task.type}">${escapeHtml(task.type)}</span>`;
        }

        card.innerHTML = `
            <div class="card-actions">
                <button class="card-action-btn edit" title="Edit task" data-action="edit">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7"/>
                        <path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z"/>
                    </svg>
                </button>
                <button class="card-action-btn delete" title="Delete task" data-action="delete">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <polyline points="3 6 5 6 21 6"/>
                        <path d="M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2"/>
                    </svg>
                </button>
            </div>
            <div class="card-header">
                <span class="card-title">${escapeHtml(task.title)}</span>
                <span class="priority-badge ${priority.class}">${priority.label}</span>
            </div>
            ${typeBadge}
            ${task.description ? `<p class="card-description">${escapeHtml(task.description)}</p>` : ''}
            ${progressHtml}
            <div class="card-meta">
                ${task.assignee ? `
                    <div class="meta-item">
                        <span class="assignee-avatar">${getInitials(task.assignee)}</span>
                        <span class="assignee-name">${escapeHtml(task.assignee)}</span>
                    </div>
                ` : ''}
                ${task.due_date ? `
                    <div class="meta-item">
                        <span class="due-date ${dueDateClass}">
                            <svg class="meta-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                <rect x="3" y="4" width="18" height="18" rx="2" ry="2"/>
                                <line x1="16" y1="2" x2="16" y2="6"/>
                                <line x1="8" y1="2" x2="8" y2="6"/>
                                <line x1="3" y1="10" x2="21" y2="10"/>
                            </svg>
                            ${formatDate(task.due_date)}
                        </span>
                    </div>
                ` : ''}
            </div>
            ${task.tags && task.tags.length > 0 ? `
                <div class="card-tags">
                    ${task.tags.map(tag => `<span class="tag">${escapeHtml(tag)}</span>`).join('')}
                </div>
            ` : ''}
        `;

        // Set progress bar width via CSSOM (CSP-safe, no inline style attribute)
        const progressFill = card.querySelector('.progress-fill[data-progress]');
        if (progressFill) {
            progressFill.style.width = progressFill.dataset.progress + '%';
        }

        // Attach event listeners
        attachCardListeners(card, task);

        return card;
    }

    /**
     * Attach event listeners to a card
     * @param {HTMLElement} card - Card element
     * @param {Object} task - Task data
     */
    function attachCardListeners(card, task) {
        // Click to view details
        card.addEventListener('click', (e) => {
            // Ignore if clicking action buttons
            if (e.target.closest('.card-action-btn')) return;
            if (isDragging) return;

            // Emit event for app to handle
            window.dispatchEvent(new CustomEvent('task:view', {
                detail: { task, cardElement: card }
            }));
        });

        // Edit button
        const editBtn = card.querySelector('[data-action="edit"]');
        if (editBtn) {
            editBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                window.dispatchEvent(new CustomEvent('task:edit', {
                    detail: { task, cardElement: card }
                }));
            });
        }

        // Delete button
        const deleteBtn = card.querySelector('[data-action="delete"]');
        if (deleteBtn) {
            deleteBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                window.dispatchEvent(new CustomEvent('task:delete', {
                    detail: { task, cardElement: card }
                }));
            });
        }

        // Keyboard navigation
        card.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                window.dispatchEvent(new CustomEvent('task:view', {
                    detail: { task, cardElement: card }
                }));
            }
        });

        // Drag events
        card.addEventListener('dragstart', handleDragStart);
        card.addEventListener('dragend', handleDragEnd);
    }

    /**
     * Update an existing card with new task data
     * @param {HTMLElement} card - Card element
     * @param {Object} task - Updated task data
     */
    function updateCard(card, task) {
        const newCard = createCard(task);
        card.replaceWith(newCard);

        // Flash animation to indicate update
        newCard.classList.add('updated');
        setTimeout(() => newCard.classList.remove('updated'), 600);

        return newCard;
    }

    /**
     * Create a skeleton loading card
     * @returns {HTMLElement}
     */
    function createSkeletonCard() {
        const card = document.createElement('div');
        card.className = 'card-skeleton';
        card.innerHTML = `
            <div class="skeleton-line title"></div>
            <div class="skeleton-line medium"></div>
            <div class="skeleton-line short"></div>
        `;
        return card;
    }

    /* ============================================
       Drag and Drop Functionality
       ============================================ */

    /**
     * Handle drag start
     * @param {DragEvent} e - Drag event
     */
    function handleDragStart(e) {
        const card = e.target.closest('.task-card');
        if (!card) return;

        isDragging = true;
        draggedCard = card;
        draggedTaskId = card.dataset.taskId;

        // Set drag data
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', draggedTaskId);

        // Add visual feedback
        requestAnimationFrame(() => {
            card.classList.add('dragging');
        });

        // Create placeholder
        dropPlaceholder = document.createElement('div');
        dropPlaceholder.className = 'drop-placeholder';
    }

    /**
     * Handle drag end
     * @param {DragEvent} e - Drag event
     */
    function handleDragEnd(e) {
        const card = e.target.closest('.task-card');
        if (card) {
            card.classList.remove('dragging');
        }

        // Clean up
        isDragging = false;
        draggedCard = null;
        draggedTaskId = null;

        if (dropPlaceholder && dropPlaceholder.parentNode) {
            dropPlaceholder.remove();
        }
        dropPlaceholder = null;

        // Remove drag-over class from all columns
        document.querySelectorAll('.column-content.drag-over').forEach(col => {
            col.classList.remove('drag-over');
        });
    }

    /**
     * Handle drag over on a column
     * @param {DragEvent} e - Drag event
     */
    function handleDragOver(e) {
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';

        const column = e.target.closest('.column-content');
        if (!column || !draggedCard) return;

        // Add visual feedback to column
        column.classList.add('drag-over');

        // Position placeholder
        const afterElement = getDragAfterElement(column, e.clientY);

        if (afterElement) {
            column.insertBefore(dropPlaceholder, afterElement);
        } else {
            column.appendChild(dropPlaceholder);
        }
    }

    /**
     * Handle drag leave from a column
     * @param {DragEvent} e - Drag event
     */
    function handleDragLeave(e) {
        const column = e.target.closest('.column-content');
        if (!column) return;

        // Only remove class if actually leaving the column
        const relatedTarget = e.relatedTarget;
        if (!column.contains(relatedTarget)) {
            column.classList.remove('drag-over');
        }
    }

    /**
     * Handle drop on a column
     * @param {DragEvent} e - Drag event
     */
    function handleDrop(e) {
        e.preventDefault();

        const column = e.target.closest('.column-content');
        if (!column || !draggedCard || !draggedTaskId) return;

        const newStatus = column.dataset.status;
        const taskId = draggedTaskId;

        // Remove placeholder
        if (dropPlaceholder && dropPlaceholder.parentNode) {
            dropPlaceholder.remove();
        }

        // Clean up column state
        column.classList.remove('drag-over');

        // Emit event for app to handle the update
        window.dispatchEvent(new CustomEvent('task:move', {
            detail: {
                taskId,
                newStatus,
                cardElement: draggedCard
            }
        }));
    }

    /**
     * Get the element to insert after based on mouse position
     * @param {HTMLElement} container - Container element
     * @param {number} y - Mouse Y position
     * @returns {HTMLElement|null}
     */
    function getDragAfterElement(container, y) {
        const draggableElements = [...container.querySelectorAll('.task-card:not(.dragging)')];

        return draggableElements.reduce((closest, child) => {
            const box = child.getBoundingClientRect();
            const offset = y - box.top - box.height / 2;

            if (offset < 0 && offset > closest.offset) {
                return { offset, element: child };
            } else {
                return closest;
            }
        }, { offset: Number.NEGATIVE_INFINITY }).element;
    }

    /**
     * Initialize drag and drop for column containers
     */
    function initDragDrop() {
        document.querySelectorAll('.column-content').forEach(column => {
            column.addEventListener('dragover', handleDragOver);
            column.addEventListener('dragleave', handleDragLeave);
            column.addEventListener('drop', handleDrop);
        });
    }

    /* ============================================
       Inline Editing
       ============================================ */

    /**
     * Enable inline editing for a card
     * @param {HTMLElement} card - Card element
     * @param {Object} task - Task data
     */
    function enableInlineEdit(card, task) {
        if (editingCardId) {
            // Cancel any existing edit
            cancelInlineEdit();
        }

        editingCardId = task.id;
        card.classList.add('editing');
        card.draggable = false;

        const titleElement = card.querySelector('.card-title');
        const currentTitle = task.title;

        // Create input
        const input = document.createElement('input');
        input.type = 'text';
        input.className = 'card-title-input';
        input.value = currentTitle;
        input.maxLength = 200;

        // Create action buttons
        const actions = document.createElement('div');
        actions.className = 'edit-actions';
        actions.innerHTML = `
            <button class="edit-btn save">Save</button>
            <button class="edit-btn cancel">Cancel</button>
        `;

        // Replace title with input
        titleElement.style.display = 'none';
        titleElement.parentNode.insertBefore(input, titleElement);

        // Insert actions at end of card
        card.appendChild(actions);

        // Focus input
        input.focus();
        input.select();

        // Event handlers
        const saveEdit = () => {
            const newTitle = input.value.trim();
            if (newTitle && newTitle !== currentTitle) {
                window.dispatchEvent(new CustomEvent('task:update', {
                    detail: {
                        taskId: task.id,
                        updates: { title: newTitle },
                        cardElement: card
                    }
                }));
            }
            cancelInlineEdit();
        };

        const cancelEdit = () => {
            cancelInlineEdit();
        };

        input.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                saveEdit();
            } else if (e.key === 'Escape') {
                cancelEdit();
            }
        });

        input.addEventListener('blur', (e) => {
            // Small delay to allow button clicks
            setTimeout(() => {
                if (editingCardId === task.id) {
                    cancelEdit();
                }
            }, 150);
        });

        actions.querySelector('.save').addEventListener('click', saveEdit);
        actions.querySelector('.cancel').addEventListener('click', cancelEdit);
    }

    /**
     * Cancel inline editing
     */
    function cancelInlineEdit() {
        if (!editingCardId) return;

        const card = document.querySelector(`[data-task-id="${editingCardId}"]`);
        if (card) {
            card.classList.remove('editing');
            card.draggable = true;

            // Remove input and actions
            const input = card.querySelector('.card-title-input');
            const actions = card.querySelector('.edit-actions');

            if (input) input.remove();
            if (actions) actions.remove();

            // Show title
            const titleElement = card.querySelector('.card-title');
            if (titleElement) {
                titleElement.style.display = '';
            }
        }

        editingCardId = null;
    }

    /* ============================================
       Public API
       ============================================ */

    return {
        createCard,
        updateCard,
        createSkeletonCard,
        initDragDrop,
        enableInlineEdit,
        cancelInlineEdit,
        formatDate,
        getDueDateClass,
        escapeHtml,
        normalizeTask,
        PRIORITIES,
        STATUS_NAMES,
        STATUS_MAP,
        STATUS_TO_API,
        TYPE_PRIORITIES,

        // Expose for external use
        get isDragging() { return isDragging; }
    };
})();

// Make available globally
window.Cards = Cards;
