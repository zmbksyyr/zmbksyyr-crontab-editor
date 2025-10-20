document.addEventListener('DOMContentLoaded', () => {
    const taskTableBody = document.getElementById('task-table-body');
    const addBtn = document.getElementById('add-task-btn');
    const saveBtn = document.getElementById('save-crontab-btn');
    const loadingDiv = document.getElementById('loading');
    const crontabListDiv = document.getElementById('crontab-list');

    let crontabEntries = []; // 用于存储当前任务列表

    // 初始加载 crontab 任务
    fetchCrontab();

    addBtn.addEventListener('click', addTask);
    saveBtn.addEventListener('click', saveCrontab);

    async function fetchCrontab() {
        loadingDiv.style.display = 'block';
        crontabListDiv.style.display = 'none';
        try {
            const response = await fetch('/api/crontab');
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            crontabEntries = await response.json();
            renderTasks();
        } catch (error) {
            console.error('Error fetching crontab:', error);
            alert('获取 Crontab 任务失败：' + error.message);
        } finally {
            loadingDiv.style.display = 'none';
            crontabListDiv.style.display = 'block';
        }
    }

    function renderTasks() {
        taskTableBody.innerHTML = ''; // 清空现有内容
        crontabEntries.forEach(entry => {
            const row = taskTableBody.insertRow();
            row.dataset.id = entry.id; // 添加 data-id 属性以便后续查找

            row.innerHTML = `
                <td><input type="checkbox" ${entry.Enabled ? 'checked' : ''} onchange="handleToggleEnable(event)"></td>
                <td><input type="text" value="${entry.Minute}" onchange="handleInputChange(event, 'minute')"></td>
                <td><input type="text" value="${entry.Hour}" onchange="handleInputChange(event, 'hour')"></td>
                <td><input type="text" value="${entry.DayOfMonth}" onchange="handleInputChange(event, 'dayOfMonth')"></td>
                <td><input type="text" value="${entry.Month}" onchange="handleInputChange(event, 'month')"></td>
                <td><input type="text" value="${entry.DayOfWeek}" onchange="handleInputChange(event, 'dayOfWeek')"></td>
                <td><input type="text" value="${entry.Command}" onchange="handleInputChange(event, 'command')" style="width: 95%;"></td>
                <td><button class="button-danger" onclick="removeTask(${entry.id})">删除</button></td>
            `;
        });
    }

    function addTask() {
        const newEntry = {
            id: Date.now(), // 简单地用时间戳作为唯一ID
            Minute: '*',
            Hour: '*',
            DayOfMonth: '*',
            Month: '*',
            DayOfWeek: '*',
            Command: '',
            RawLine: '', // 新增任务没有原始行
            Comment: '',
            Enabled: true
        };
        crontabEntries.push(newEntry);
        renderTasks();
    }

    function removeTask(id) {
        if (!confirm('确定要删除此任务吗？')) {
            return;
        }
        crontabEntries = crontabEntries.filter(entry => entry.id !== id);
        renderTasks();
    }

    // 全局函数，通过事件委托处理 input 变化
    window.handleInputChange = (event, field) => {
        const row = event.target.closest('tr');
        const id = parseInt(row.dataset.id);
        const entry = crontabEntries.find(e => e.id === id);
        if (entry) {
            entry[field] = event.target.value;
        }
    };

    window.handleToggleEnable = (event) => {
        const row = event.target.closest('tr');
        const id = parseInt(row.dataset.id);
        const entry = crontabEntries.find(e => e.id === id);
        if (entry) {
            entry.Enabled = event.target.checked;
        }
    };

    async function saveCrontab() {
        if (!confirm('确定要保存 Crontab 吗？此操作将修改您的系统 Crontab。')) {
            return;
        }

        loadingDiv.style.display = 'block';
        crontabListDiv.style.display = 'none';
        try {
            const response = await fetch('/api/crontab', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(crontabEntries)
            });
            if (!response.ok) {
                const errorText = await response.text();
                throw new Error(`HTTP error! status: ${response.status}, message: ${errorText}`);
            }
            alert('Crontab 保存成功！');
            // 保存成功后重新获取并渲染，以确保显示最新的状态（包括ID和RawLine）
            await fetchCrontab();
        } catch (error) {
            console.error('Error saving crontab:', error);
            alert('保存 Crontab 失败：' + error.message);
        } finally {
            loadingDiv.style.display = 'none';
            crontabListDiv.style.display = 'block';
        }
    }
});
