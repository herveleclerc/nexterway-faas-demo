// Client JavaScript pour le dashboard Nexterway FaaS Demo
document.addEventListener("DOMContentLoaded", () => {
    const taskTableBody = document.getElementById("taskTableBody");
    const countTotal = document.getElementById("countTotal");
    const countPending = document.getElementById("countPending");
    const countProcessing = document.getElementById("countProcessing");
    const countCompleted = document.getElementById("countCompleted");
    const taskForm = document.getElementById("taskForm");
    const refreshBtn = document.getElementById("refreshBtn");
    const autoRefreshToggle = document.getElementById("autoRefreshToggle");

    let isAutoRefresh = true;
    let pollInterval = null;

    async function loadTasks() {
        try {
            const res = await fetch("/api/tasks");
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const json = await res.json();

            updateStats(json.stats || {});
            renderTasks(json.data || []);
        } catch (err) {
            console.error("Erreur chargement tâches:", err);
            showToast("Erreur lors de la récupération des tâches", "error");
        }
    }

    function updateStats(stats) {
        if (countTotal) countTotal.textContent = stats.total || 0;
        if (countPending) countPending.textContent = stats.pending || 0;
        if (countProcessing) countProcessing.textContent = stats.processing || 0;
        if (countCompleted) countCompleted.textContent = stats.completed || 0;
    }

    function renderTasks(tasks) {
        if (!taskTableBody) return;
        if (tasks.length === 0) {
            taskTableBody.innerHTML = `
                <tr>
                    <td colspan="6" class="px-6 py-8 text-center text-sm text-gray-500">
                        Aucune tâche trouvée dans PostgreSQL. Créez-en une avec le formulaire ci-dessus !
                    </td>
                </tr>
            `;
            return;
        }

        taskTableBody.innerHTML = tasks.map(task => {
            const badgeClass = getBadgeClass(task.status);
            const dateStr = new Date(task.created_at).toLocaleString();
            const isPending = task.status === "PENDING";

            return `
                <tr class="hover:bg-slate-50 transition-colors animate-fade-in border-b border-gray-100">
                    <td class="px-6 py-4 font-mono text-xs text-gray-500">${task.id.substring(0, 8)}...</td>
                    <td class="px-6 py-4">
                        <div class="text-sm font-semibold text-gray-900">${escapeHtml(task.title)}</div>
                        <div class="text-xs text-gray-500">${escapeHtml(task.description || "-")}</div>
                    </td>
                    <td class="px-6 py-4">
                        <span class="text-xs font-semibold px-2 py-0.5 rounded ${getPriorityClass(task.priority)}">
                            ${task.priority}
                        </span>
                    </td>
                    <td class="px-6 py-4">
                        <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${badgeClass}">
                            ${task.status}
                        </span>
                    </td>
                    <td class="px-6 py-4 text-xs text-gray-500">${dateStr}</td>
                    <td class="px-6 py-4 text-right">
                        ${isPending ? `
                            <button onclick="processTask('${task.id}')"
                                class="inline-flex items-center px-3 py-1 bg-indigo-600 hover:bg-indigo-700 text-white text-xs font-medium rounded shadow-sm transition">
                                <svg class="w-3.5 h-3.5 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z"/>
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
                                </svg>
                                Traiter (FaaS)
                            </button>
                        ` : `
                            <span class="text-xs text-gray-400 italic">Terminé</span>
                        `}
                    </td>
                </tr>
            `;
        }).join("");
    }

    window.processTask = async function(id) {
        try {
            showToast("Déclenchement de la fonction Fission...", "info");
            const res = await fetch(`/api/tasks?action=process&id=${id}`, {
                method: "POST",
                headers: { "Content-Type": "application/json" }
            });
            const json = await res.json();
            if (!res.ok) throw new Error(json.message || "Erreur de traitement");

            showToast("Tâche traitée par Fission et mise à jour dans PostgreSQL !", "success");
            loadTasks();
        } catch (err) {
            console.error("Erreur process:", err);
            showToast(err.message, "error");
        }
    };

    if (taskForm) {
        taskForm.addEventListener("submit", async (e) => {
            e.preventDefault();
            const submitBtn = taskForm.querySelector("button[type='submit']");
            submitBtn.disabled = true;

            const payload = {
                title: document.getElementById("taskTitle").value,
                description: document.getElementById("taskDesc").value,
                priority: document.getElementById("taskPriority").value
            };

            try {
                const res = await fetch("/api/tasks", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify(payload)
                });
                const json = await res.json();
                if (!res.ok) throw new Error(json.message || "Erreur création");

                showToast("Nouvelle tâche insérée dans PostgreSQL via Fission", "success");
                taskForm.reset();
                loadTasks();
            } catch (err) {
                console.error("Erreur form:", err);
                showToast(err.message, "error");
            } finally {
                submitBtn.disabled = false;
            }
        });
    }

    if (refreshBtn) {
        refreshBtn.addEventListener("click", () => {
            loadTasks();
            showToast("Données actualisées", "info");
        });
    }

    if (autoRefreshToggle) {
        autoRefreshToggle.addEventListener("change", (e) => {
            isAutoRefresh = e.target.checked;
            togglePolling();
        });
    }

    function togglePolling() {
        if (pollInterval) clearInterval(pollInterval);
        if (isAutoRefresh) {
            pollInterval = setInterval(loadTasks, 4000);
        }
    }

    function getBadgeClass(status) {
        switch (status) {
            case "PENDING": return "badge-pending";
            case "PROCESSING": return "badge-processing";
            case "COMPLETED": return "badge-completed";
            case "FAILED": return "badge-failed";
            default: return "bg-gray-100 text-gray-800";
        }
    }

    function getPriorityClass(priority) {
        switch (priority) {
            case "URGENT": return "bg-red-100 text-red-800";
            case "HIGH": return "bg-orange-100 text-orange-800";
            case "MEDIUM": return "bg-blue-100 text-blue-800";
            case "LOW": return "bg-gray-100 text-gray-800";
            default: return "bg-gray-100 text-gray-800";
        }
    }

    function showToast(message, type = "info") {
        const toast = document.createElement("div");
        const colors = {
            success: "bg-emerald-600 text-white",
            error: "bg-rose-600 text-white",
            info: "bg-slate-800 text-white"
        };
        toast.className = `toast px-4 py-2.5 rounded-lg shadow-lg text-sm font-medium ${colors[type] || colors.info} animate-fade-in flex items-center space-x-2`;
        toast.innerHTML = `<span>${escapeHtml(message)}</span>`;
        document.body.appendChild(toast);

        setTimeout(() => {
            toast.style.opacity = "0";
            setTimeout(() => toast.remove(), 300);
        }, 3500);
    }

    function escapeHtml(str) {
        return (str || "").replace(/[&<>"']/g, m => ({
            "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;"
        })[m]);
    }

    // Premier chargement
    loadTasks();
    togglePolling();
});
