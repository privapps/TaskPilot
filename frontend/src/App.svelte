<script>
  import { onMount, onDestroy } from 'svelte';
  import { marked } from 'marked';
  import { GetJobs, CreateJob, DeleteJob, GetJobHistoryPage, UpdateJob, PauseJob, ResumeJob, DeleteJobHistory, DeleteAllJobHistory, DuplicateJob } from '../wailsjs/go/services/JobService';
  import { ExportJobsWithDialog, PrepareImportWithDialog, ImportJobsFromFile, TriggerJob } from '../wailsjs/go/main/App';
  import { BrowserOpenURL, ClipboardSetText } from '../wailsjs/runtime/runtime';
  import { models } from '../wailsjs/go/models';
  import DefaultsModal from './DefaultsModal.svelte';
  
  let jobs = [];
  let showModal = false;
  let showHistoryModal = false;
  let showDefaultsModal = false;
  let selectedJobHistory = [];
  let selectedJobName = '';
  let selectedJobId = '';
  const HISTORY_PAGE_SIZE = 20;
  let historyOffset = 0;
  let hasMoreHistory = false;
  let loadingMoreHistory = false;
  let historyModalMaximized = false;
  let copiedHistoryId = '';
  let copyTimeout = null;
  let editingJob = null;
  let refreshInterval;
  let newJob = {
    name: '',
    command: '',
    directory: '',
    schedule: '',
    sound_file: '',
    on_success_cmd: '',
    schedule_type: 'cron',
    delay_minutes: null,
    run_at: null,
    disable_macos_sleep_prevention: false
  };
  let loading = false;
  let error = '';
  let successMessage = '';
  let successTimeout = null; // Track timeout to prevent race conditions
  let scheduleType = 'cron'; // For form control
  let wailsReady = false;
  
  // Confirmation dialog state
  let showConfirmDialog = false;
  let confirmCallback = null;
  let confirmMessage = '';

  // Wait for Wails runtime to be ready
  function waitForWails() {
    return new Promise((resolve) => {
      const checkReady = () => {
        // Check if Wails API exists
        if (!window.go || !window.go.services || !window.go.services.JobService) {
          return false;
        }
        
        // Check if WebSocket is connected (for dev mode)
        if (window.runtime && window.runtime.websocket) {
          const ws = window.runtime.websocket;
          if (ws.readyState !== WebSocket.OPEN) {
            return false;
          }
        }
        
        return true;
      };
      
      if (checkReady()) {
        wailsReady = true;
        resolve();
      } else {
        const checkInterval = setInterval(() => {
          if (checkReady()) {
            clearInterval(checkInterval);
            wailsReady = true;
            resolve();
          }
        }, 100);
        
        // Timeout after 15 seconds
        setTimeout(() => {
          clearInterval(checkInterval);
          console.error('Wails runtime did not initialize in time');
          error = 'Application failed to initialize. Please restart.';
          resolve();
        }, 15000);
      }
    });
  }

  onMount(async () => {
    await waitForWails();
    // Add a small delay to ensure WebSocket is fully ready
    await new Promise(resolve => setTimeout(resolve, 200));
    await loadJobs();
    // Auto-refresh every 5 seconds to see status updates
    refreshInterval = setInterval(loadJobs, 5000);
    
    // Add ESC key handler for closing modals
    window.addEventListener('keydown', handleKeyDown);
  });

  onDestroy(() => {
    if (refreshInterval) {
      clearInterval(refreshInterval);
    }
    if (copyTimeout) {
      clearTimeout(copyTimeout);
    }
    window.removeEventListener('keydown', handleKeyDown);
  });

  function handleKeyDown(event) {
    if (event.key === 'Escape') {
      if (showConfirmDialog) {
        handleConfirmNo();
      } else if (showHistoryModal) {
        closeHistoryModal();
      } else if (showModal) {
        closeModal();
      }
    }
  }

  async function loadJobs() {
    if (!wailsReady) return;
    
    try {
      error = '';
      let loadedJobs = await GetJobs() || [];
      
      // Sort jobs:
      // 1. Running jobs first
      // 2. Active (not paused) jobs
      // 3. Within each group, sort by most recently executed (descending)
      jobs = loadedJobs.sort((a, b) => {
        // Priority 1: Running jobs
        const aRunning = a.status === 'running' ? 1 : 0;
        const bRunning = b.status === 'running' ? 1 : 0;
        if (aRunning !== bRunning) return bRunning - aRunning;
        
        // Priority 2: Active (not paused) jobs
        const aActive = !a.paused ? 1 : 0;
        const bActive = !b.paused ? 1 : 0;
        if (aActive !== bActive) return bActive - aActive;
        
        // Priority 3: Sort by last execution time (most recent first)
        // Jobs with execution history come before jobs without
        const aTime = a.last_run_at ? a.last_run_at : (a.last_scheduled_at || 0);
        const bTime = b.last_run_at ? b.last_run_at : (b.last_scheduled_at || 0);
        
        if (aTime !== bTime) return bTime - aTime; // Descending order
        
        // Finally, sort alphabetically by name
        return a.name.localeCompare(b.name);
      });
    } catch (err) {
      error = 'Failed to load jobs: ' + err;
      console.error(err);
    }
  }

  // Helper function to show custom confirmation dialog
  function showConfirm(message) {
    return new Promise((resolve) => {
      confirmMessage = message;
      confirmCallback = resolve;
      showConfirmDialog = true;
    });
  }
  
  function handleConfirmYes() {
    showConfirmDialog = false;
    if (confirmCallback) {
      confirmCallback(true);
      confirmCallback = null;
    }
  }
  
  function handleConfirmNo() {
    showConfirmDialog = false;
    if (confirmCallback) {
      confirmCallback(false);
      confirmCallback = null;
    }
  }

  async function viewHistory(job) {
    try {
      selectedJobName = job.name;
      selectedJobId = job.id;
      selectedJobHistory = [];
      historyOffset = 0;
      hasMoreHistory = false;
      historyModalMaximized = false;
      showHistoryModal = true;
      await loadHistoryPage();
    } catch (err) {
      error = 'Failed to load history: ' + err;
      console.error(err);
    }
  }

  function closeHistoryModal() {
    showHistoryModal = false;
    selectedJobHistory = [];
    selectedJobName = '';
    selectedJobId = '';
    historyOffset = 0;
    hasMoreHistory = false;
    loadingMoreHistory = false;
    historyModalMaximized = false;
    copiedHistoryId = '';
  }

  async function loadHistoryPage(append = false) {
    if (!selectedJobId || (append && loadingMoreHistory)) {
      return;
    }

    if (append) {
      loadingMoreHistory = true;
    }

    const offset = append ? historyOffset : 0;
    try {
      const page = await GetJobHistoryPage(selectedJobId, HISTORY_PAGE_SIZE, offset) || [];
      if (append) {
        selectedJobHistory = [...selectedJobHistory, ...page];
      } else {
        selectedJobHistory = page;
      }
      historyOffset = offset + page.length;
      hasMoreHistory = page.length === HISTORY_PAGE_SIZE;
    } catch (err) {
      error = 'Failed to load history: ' + err;
      console.error('History page error:', err);
    } finally {
      loadingMoreHistory = false;
    }
  }

  function formatTimestamp(ts) {
    if (!ts) return 'N/A';
    // Timestamp is now a number (int64), multiply by 1000 for milliseconds
    const date = new Date(ts * 1000);
    return date.toLocaleString();
  }

  function formatDuration(ms) {
    if (!ms) return 'N/A';
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  }

  function renderMarkdown(content) {
    if (!content) {
      return '<p class="text-gray-500 text-sm italic">No output</p>';
    }

    return marked.parse(String(content), {
      gfm: true,
      breaks: true
    });
  }

  // Converts a Unix timestamp (seconds) into a local datetime string (YYYY-MM-DDTHH:mm)
  // suitable for datetime-local inputs. Adjusts for the local timezone offset so the
  // displayed time matches what the user originally scheduled.
  function convertUnixToLocalDatetime(unixSeconds) {
    const date = new Date(unixSeconds * 1000); // Convert seconds to milliseconds
    // Adjust for local timezone offset to get the wall-clock time
    const adjustedTime = new Date(date.getTime() - date.getTimezoneOffset() * 60000);
    return adjustedTime.toISOString().slice(0, 16); // Keep only YYYY-MM-DDTHH:mm
  }

  // Shows a success message that auto-clears after 3 seconds.
  // Cancels any pending clear to avoid race conditions when messages fire in quick succession.
  function showSuccess(message) {
    successMessage = message;
    if (successTimeout) clearTimeout(successTimeout);
    successTimeout = setTimeout(() => { successMessage = ''; successTimeout = null; }, 3000);
  }

  async function handleCreateJob() {
    if (!wailsReady) {
      error = 'Application is still initializing. Please wait...';
      return;
    }
    
    if (!newJob.name || !newJob.command) {
      error = 'Name and command are required';
      return;
    }

    // Validate based on schedule type
    if (scheduleType === 'cron' && !newJob.schedule) {
      error = 'Cron schedule is required';
      return;
    }
    if (scheduleType === 'delay' && (!newJob.delay_minutes || newJob.delay_minutes <= 0)) {
      error = 'Delay must be a positive number of minutes';
      return;
    }
    if (scheduleType === 'datetime' && !newJob.run_at) {
      error = 'Datetime is required';
      return;
    }
    // Datetime parsing and future-time policy are owned by the backend schedule
    // semantics module so Wails, REST, and MCP callers share one rule set.
    // No validation is needed for immediate - it runs right away.

    try {
      loading = true;
      error = '';
      
      // Prepare job object based on schedule type
      const jobData = {
        name: newJob.name,
        command: newJob.command,
        directory: newJob.directory,
        schedule: newJob.schedule,
        sound_file: newJob.sound_file,
        on_success_cmd: newJob.on_success_cmd,
        schedule_type: scheduleType,
        delay_minutes: null,
        run_at: null,
        disable_macos_sleep_prevention: newJob.disable_macos_sleep_prevention,
        paused: false  // When editing, unpause the job (especially important for one-time jobs)
      };
      
      if (scheduleType === 'delay') {
        // For delay, send delay_minutes; backend will calculate run_at
        jobData.delay_minutes = parseInt(newJob.delay_minutes);
        jobData.run_at = null;
      } else if (scheduleType === 'datetime') {
        // Convert datetime-local to unix timestamp
        const dt = new Date(newJob.run_at);
        jobData.run_at = Math.floor(dt.getTime() / 1000);
        jobData.delay_minutes = null;
      } else if (scheduleType === 'immediate') {
        // Immediate execution - clear schedule fields
        jobData.schedule = 'immediate';
        jobData.schedule_type = 'immediate';
        jobData.delay_minutes = null;
        jobData.run_at = null;
      } else {
        // Cron type - use the schedule field
        jobData.schedule_type = 'cron';
        jobData.delay_minutes = null;
        jobData.run_at = null;
      }
      
      const job = models.Job.createFrom(jobData);
      
      if (editingJob) {
        job.id = editingJob.id;
        await UpdateJob(job);
      } else {
        await CreateJob(job);
      }
      
      // Reset form
      newJob = {
        name: '',
        command: '',
        directory: '',
        schedule: '',
        sound_file: '',
        on_success_cmd: '',
        schedule_type: 'cron',
        delay_minutes: null,
        run_at: null,
        disable_macos_sleep_prevention: false
      };
      scheduleType = 'cron';
      editingJob = null;
      
      showModal = false;
      await loadJobs();
    } catch (err) {
      error = (editingJob ? 'Failed to update job: ' : 'Failed to create job: ') + err;
      console.error(err);
    } finally {
      loading = false;
    }
  }

  async function handleDeleteJob(jobId) {
    const confirmed = await showConfirm('Are you sure you want to delete this job?');
    if (!confirmed) {
      return;
    }

    try {
      loading = true;
      error = '';
      await DeleteJob(jobId);
      await loadJobs();
    } catch (err) {
      error = 'Failed to delete job: ' + err;
      console.error(err);
    } finally {
      loading = false;
    }
  }

  async function handleDuplicateJob(jobId) {
    try {
      error = '';
      await DuplicateJob(jobId);
      await loadJobs();
    } catch (err) {
      error = 'Failed to duplicate job: ' + err;
      console.error(err);
    }
  }

  async function handlePauseJob(jobId) {
    try {
      error = '';
      await PauseJob(jobId);
      await loadJobs();
    } catch (err) {
      error = 'Failed to pause job: ' + err;
      console.error(err);
    }
  }

  async function handleResumeJob(jobId) {
    try {
      error = '';
      await ResumeJob(jobId);
      await loadJobs();
    } catch (err) {
      error = 'Failed to resume job: ' + err;
      console.error(err);
    }
  }

  async function handleTriggerJob(jobId) {
    try {
      error = '';
      const triggerJob = jobs.find(j => j.id === jobId);
      if (!triggerJob) {
        console.warn(`Job with ID ${jobId} not found in local list.`);
      }
      await TriggerJob(jobId);
      const jobName = triggerJob?.name || `ID: ${jobId}`;
      showSuccess(`Job "${jobName}" triggered successfully`);
      await loadJobs();
    } catch (err) {
      error = 'Failed to trigger job: ' + err;
      console.error(err);
    }
  }

  function handleEditJob(job) {
    editingJob = job;
    scheduleType = job.schedule_type || 'cron';
    
    // Populate form with job data
    newJob = {
      name: job.name,
      command: job.command,
      directory: job.directory || '',
      schedule: job.schedule || '',
      sound_file: job.sound_file || '',
      on_success_cmd: job.on_success_cmd || '',
      schedule_type: job.schedule_type || 'cron',
      delay_minutes: job.delay_minutes || null,
      run_at: job.run_at ? convertUnixToLocalDatetime(job.run_at) : null,
      disable_macos_sleep_prevention: job.disable_macos_sleep_prevention || false
    };
    
    showModal = true;
    error = '';
  }

  async function handleExport() {
    try {
      error = '';
      await ExportJobsWithDialog();
      showSuccess('Jobs exported successfully!');
    } catch (err) {
      if (err) { // Don't show error if user cancelled
        error = 'Failed to export jobs: ' + err;
        console.error(err);
      }
    }
  }

  async function handleImport() {
    try {
      error = '';
      
      // Open file dialog and check for defaults
      const result = await PrepareImportWithDialog();
      
      // User cancelled the dialog
      if (result.cancelled) {
        return;
      }
      
      // If file contains defaults, show confirmation dialog
      if (result.hasDefaults) {
        const confirmed = await new Promise((resolve) => {
          confirmMessage = 'This import will update your default settings. Continue?';
          confirmCallback = resolve;
          showConfirmDialog = true;
        });
        
        if (!confirmed) {
          return; // User cancelled defaults import
        }
      }
      
      // Proceed with import
      await ImportJobsFromFile(result.filepath);
      
      if (result.hasDefaults) {
        showSuccess('Jobs and defaults imported successfully!');
      } else {
        showSuccess('Jobs imported successfully!');
      }
      
      await loadJobs();
    } catch (err) {
      if (err) { // Don't show error if user cancelled
        error = 'Failed to import jobs: ' + err;
        console.error(err);
      }
    }
  }

  function handleDefaultsSaved() {
    showSuccess('Default settings saved successfully!');
  }

  async function handleDeleteHistory(historyId) {
    console.log('handleDeleteHistory called with:', historyId, 'type:', typeof historyId);
    
    const confirmed = await showConfirm('Delete this history entry?');
    if (!confirmed) {
      return;
    }

    try {
      console.log('Calling DeleteJobHistory with string:', String(historyId));
      await DeleteJobHistory(String(historyId));
      selectedJobHistory = selectedJobHistory.filter((entry) => String(entry.id) !== String(historyId));
      historyOffset = selectedJobHistory.length;
    } catch (err) {
      error = 'Failed to delete history: ' + err;
      console.error('Delete history error:', err);
    }
  }

  async function handleCopyHistory(entry) {
    try {
      const copied = await ClipboardSetText(entry.output || '');
      if (copied === false) {
        throw new Error('Clipboard write was rejected');
      }
      copiedHistoryId = String(entry.id);
      if (copyTimeout) {
        clearTimeout(copyTimeout);
      }
      copyTimeout = setTimeout(() => {
        copiedHistoryId = '';
      }, 1500);
    } catch (err) {
      error = 'Failed to copy history output: ' + err;
      console.error('Copy history error:', err);
    }
  }

  function handleHistoryOutputClick(event) {
    const target = event.target;
    const link = target instanceof Element ? target.closest('a') : null;
    if (!link) {
      return;
    }

    const href = link.getAttribute('href') || '';
    if (!/^https?:\/\//i.test(href)) {
      return;
    }

    event.preventDefault();
    BrowserOpenURL(href);
  }

  async function handleClearAllHistory() {
    console.log('handleClearAllHistory called with jobID:', selectedJobId);
    
    const confirmed = await showConfirm('Delete all history for this job? This cannot be undone.');
    if (!confirmed) {
      return;
    }

    try {
      await DeleteAllJobHistory(selectedJobId);
      selectedJobHistory = [];
      closeHistoryModal();
    } catch (err) {
      error = 'Failed to clear history: ' + err;
      console.error('Clear all history error:', err);
    }
  }

  function formatScheduleDisplay(job) {
    if (job.schedule_type === 'delay') {
      if (job.run_at) {
        const runDate = new Date(job.run_at * 1000);
        const now = new Date();
        const diffMin = Math.floor((runDate - now) / 60000);
        if (diffMin > 0) {
          return `In ${diffMin} min (${runDate.toLocaleTimeString()})`;
        } else {
          return runDate.toLocaleString();
        }
      }
      return `Delay: ${job.delay_minutes || '?'} min`;
    } else if (job.schedule_type === 'datetime') {
      if (job.run_at) {
        const runDate = new Date(job.run_at * 1000);
        return `Once at ${runDate.toLocaleString()}`;
      }
      return 'One-time';
    } else if (job.schedule_type === 'immediate') {
      return 'Run immediately';
    }
    // Default to cron
    return job.schedule || 'N/A';
  }

  function formatLastRun(job) {
    if (!job?.last_run_at) {
      if (job?.last_result === 'missed' && job?.last_scheduled_at) {
        const scheduledRun = new Date(job.last_scheduled_at * 1000);
        return `Missed at ${scheduledRun.toLocaleTimeString()}`;
      }
      return 'Never';
    }
    
    const lastRun = new Date(job.last_run_at * 1000);
    const now = new Date();
    const diffMs = now - lastRun;
    const diffHours = diffMs / (1000 * 60 * 60);
    
    // If less than 24 hours, show relative time
    if (diffHours < 24) {
      const diffMinutes = Math.floor(diffMs / (1000 * 60));
      if (diffMinutes < 1) {
        return 'Just now';
      } else if (diffMinutes < 60) {
        return `${diffMinutes} min ago`;
      } else {
        const hours = Math.floor(diffHours);
        return `${hours} hour${hours > 1 ? 's' : ''} ago`;
      }
    }
    
    // If more than 24 hours, show absolute date/time
    return lastRun.toLocaleString();
  }

  function openModal() {
    editingJob = null;
    scheduleType = 'cron';
    newJob = {
      name: '',
      command: '',
      directory: '',
      schedule: '',
      sound_file: '',
      on_success_cmd: '',
      schedule_type: 'cron',
      delay_minutes: null,
      run_at: null,
      disable_macos_sleep_prevention: false
    };
    showModal = true;
    error = '';
  }

  function closeModal() {
    showModal = false;
    editingJob = null;
    error = '';
  }
</script>

<main class="min-h-screen bg-gray-900 text-white p-8">
  <div class="max-w-6xl mx-auto">
    {#if !wailsReady}
      <div class="flex items-center justify-center h-screen">
        <div class="text-center">
          <div class="text-6xl mb-4">⏳</div>
          <div class="text-xl">Initializing TaskPilot...</div>
        </div>
      </div>
    {:else}
    <div class="flex justify-between items-center mb-8">
      <h1 class="text-4xl font-bold">TaskPilot</h1>
      <div class="flex gap-3">
        <button
          onclick={handleExport}
          class="px-4 py-2 bg-gray-700 hover:bg-gray-600 rounded-md font-medium transition-colors"
        >
          📤 Export
        </button>
        <button
          onclick={handleImport}
          class="px-4 py-2 bg-gray-700 hover:bg-gray-600 rounded-md font-medium transition-colors"
        >
          📥 Import
        </button>
        <button
          onclick={() => showDefaultsModal = true}
          class="px-4 py-2 bg-gray-700 hover:bg-gray-600 rounded-md font-medium transition-colors"
        >
          ⚙️ Defaults
        </button>
        <button
          onclick={openModal}
          class="px-6 py-2 bg-blue-600 hover:bg-blue-700 rounded-md font-medium transition-colors"
        >
          + New Job
        </button>
      </div>
    </div>

    {#if error}
      <div class="mb-4 p-4 bg-red-900/30 border border-red-700 rounded-md text-red-300">
        {error}
      </div>
    {/if}

    {#if successMessage}
      <div class="mb-4 p-4 bg-green-900/30 border border-green-700 rounded-md text-green-300">
        {successMessage}
      </div>
    {/if}

    {#if loading}
      <div class="text-center py-12">
        <div class="inline-block animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-blue-500"></div>
        <p class="mt-4 text-gray-400">Loading...</p>
      </div>
    {:else if jobs.length === 0}
      <div class="text-center py-12">
        <p class="text-gray-400 text-lg">No jobs yet. Create your first job to get started!</p>
      </div>
    {:else}
      <div class="grid gap-4">
        {#each jobs as job (job.id)}
          <div class={`bg-gray-800 rounded-lg p-6 shadow-lg hover:shadow-xl transition-shadow ${job.paused ? 'opacity-60' : ''}`}>
            <div class="flex justify-between items-start mb-3">
              <div class="flex-1">
                <div class="flex items-center gap-3 mb-2">
                  <h3 class="text-xl font-semibold">{job.name}</h3>
                  {#if job.paused}
                    <span class="px-2 py-1 text-xs rounded-full bg-gray-700 text-gray-300 border border-gray-600">
                      ⏸ paused
                    </span>
                  {/if}
                  <span class={`px-2 py-1 text-xs rounded-full ${
                    job.status === 'running' ? 'bg-yellow-900/30 text-yellow-400 border border-yellow-700' :
                    'bg-gray-700 text-gray-400'
                  }`}>
                    {job.status}
                  </span>
                  {#if job.last_result}
                    <span class={`px-2 py-1 text-xs rounded-full ${
                      job.last_result === 'success' ? 'bg-green-900/30 text-green-400 border border-green-700' :
                      job.last_result === 'failed' ? 'bg-red-900/30 text-red-400 border border-red-700' :
                      job.last_result === 'missed' ? 'bg-yellow-900/30 text-yellow-400 border border-yellow-700' :
                      'bg-gray-700 text-gray-400'
                    }`}>
                      {job.last_result}
                    </span>
                  {/if}
                </div>
              </div>
              <div class="flex gap-2">
                {#if job.paused}
                  <button
                    onclick={() => handleResumeJob(job.id)}
                    class="px-4 py-2 bg-green-600 hover:bg-green-700 rounded-md text-sm font-medium transition-colors"
                    title="Resume job"
                  >
                    ▶ Resume
                  </button>
                {:else}
                  <button
                    onclick={() => handlePauseJob(job.id)}
                    class="px-4 py-2 bg-yellow-600 hover:bg-yellow-700 rounded-md text-sm font-medium transition-colors"
                    title="Pause job"
                  >
                    ⏸ Pause
                  </button>
                {/if}
                <button
                  onclick={() => handleTriggerJob(job.id)}
                  class="px-4 py-2 bg-orange-600 hover:bg-orange-700 rounded-md text-sm font-medium transition-colors"
                  title="Run this job immediately"
                >
                  ▶️ Trigger
                </button>
                <button
                  onclick={() => handleEditJob(job)}
                  class="px-4 py-2 bg-purple-600 hover:bg-purple-700 rounded-md text-sm font-medium transition-colors"
                >
                  Edit
                </button>
                <button
                  onclick={() => viewHistory(job)}
                  class="px-4 py-2 bg-blue-600 hover:bg-blue-700 rounded-md text-sm font-medium transition-colors"
                >
                  History
                </button>
                <button
                  onclick={() => handleDuplicateJob(job.id)}
                  class="px-4 py-2 bg-teal-600 hover:bg-teal-700 rounded-md text-sm font-medium transition-colors"
                  title="Duplicate job (paused)"
                >
                  📋 Duplicate
                </button>
                <button
                  onclick={() => handleDeleteJob(job.id)}
                  class="px-4 py-2 bg-red-600 hover:bg-red-700 rounded-md text-sm font-medium transition-colors"
                >
                  Delete
                </button>
              </div>
            </div>
            <div class="border-t border-gray-700 pt-3 pb-3">
              <p class="text-gray-400 font-mono text-sm break-all">{job.command}</p>
            </div>
            <div class="flex justify-between items-start border-t border-gray-700 pt-3">
              <div class="flex flex-wrap gap-x-6 gap-y-2 text-sm text-gray-500">
                <div>Schedule: <span class="text-gray-300">{formatScheduleDisplay(job)}</span></div>
                {#if job.directory}
                  <div>Directory: <span class="text-gray-300">{job.directory}</span></div>
                {/if}
                {#if job.sound_file}
                  <div>🔊 Sound enabled</div>
                {/if}
              </div>
              <div class="text-sm text-gray-500 text-right ml-4">
                <span class="text-gray-400">Last run:</span> <span class="text-gray-300">{formatLastRun(job)}</span>
              </div>
            </div>
          </div>
        {/each}
      </div>
    {/if}

  <!-- Modal -->
  {#if showModal}
    <div class="fixed inset-0 bg-black/50 flex items-center justify-center p-4 z-50" onclick={closeModal}>
      <div class="bg-gray-800 rounded-lg p-6 max-w-2xl w-full shadow-xl max-h-[90vh] overflow-y-auto" onclick={(e) => e.stopPropagation()}>
        <h2 class="text-2xl font-semibold mb-6">{editingJob ? 'Edit Job' : 'Create New Job'}</h2>
        
        {#if error}
          <div class="mb-4 p-4 bg-red-900/30 border border-red-700 rounded-md text-red-300">
            {error}
          </div>
        {/if}
        
        <form onsubmit={(e) => { e.preventDefault(); handleCreateJob(); }} class="space-y-4">
          <div>
            <label for="name" class="block text-sm font-medium mb-2">Job Name *</label>
            <input
              id="name"
              type="text"
              bind:value={newJob.name}
              class="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
              placeholder="My Daily Backup"
              required
            />
          </div>

          <div>
            <label for="command" class="block text-sm font-medium mb-2">Command *</label>
            <input
              id="command"
              type="text"
              bind:value={newJob.command}
              class="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 font-mono text-sm"
              placeholder="npm run build"
              required
            />
          </div>

          <!-- Schedule Type Selector -->
          <div>
            <label class="block text-sm font-medium mb-2">Schedule Type *</label>
            <div class="flex gap-4 flex-wrap">
              <label class="flex items-center">
                <input
                  type="radio"
                  bind:group={scheduleType}
                  value="cron"
                  class="mr-2"
                />
                <span>Cron (Recurring)</span>
              </label>
              <label class="flex items-center">
                <input
                  type="radio"
                  bind:group={scheduleType}
                  value="delay"
                  class="mr-2"
                />
                <span>Delay (In X minutes)</span>
              </label>
              <label class="flex items-center">
                <input
                  type="radio"
                  bind:group={scheduleType}
                  value="datetime"
                  class="mr-2"
                />
                <span>Specific Date/Time</span>
              </label>
              <label class="flex items-center">
                <input
                  type="radio"
                  bind:group={scheduleType}
                  value="immediate"
                  class="mr-2"
                />
                <span>Run Immediately</span>
              </label>
            </div>
          </div>

          <!-- Conditional Schedule Inputs -->
          {#if scheduleType === 'cron'}
            <div>
              <label for="schedule" class="block text-sm font-medium mb-2">Cron Expression *</label>
              <input
                id="schedule"
                type="text"
                bind:value={newJob.schedule}
                class="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 font-mono text-sm"
                placeholder="* * * * * (every minute)"
                required
              />
              <p class="text-xs text-gray-400 mt-1">Format: minute hour day month weekday</p>
            </div>
          {:else if scheduleType === 'delay'}
            <div>
              <label for="delay_minutes" class="block text-sm font-medium mb-2">Delay (minutes) *</label>
              <input
                id="delay_minutes"
                type="number"
                bind:value={newJob.delay_minutes}
                class="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                placeholder="30"
                min="1"
                required
              />
              <p class="text-xs text-gray-400 mt-1">Job will run once after this many minutes</p>
            </div>
          {:else if scheduleType === 'datetime'}
            <div>
              <label for="run_at" class="block text-sm font-medium mb-2">Run At (Date & Time) *</label>
              <input
                id="run_at"
                type="datetime-local"
                bind:value={newJob.run_at}
                class="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                required
              />
              <p class="text-xs text-gray-400 mt-1">Job will run once at this specific time (timezone: {Intl.DateTimeFormat().resolvedOptions().timeZone})</p>
            </div>
          {:else if scheduleType === 'immediate'}
            <div class="text-sm text-gray-400 p-4 bg-gray-700 rounded border border-gray-600">
              <p>✓ This job will execute immediately once created.</p>
              <p class="mt-1">No schedule configuration needed.</p>
            </div>
          {/if}

          <div>
            <label for="directory" class="block text-sm font-medium mb-2">Working Directory</label>
            <input
              id="directory"
              type="text"
              bind:value={newJob.directory}
              class="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
              placeholder="/path/to/directory"
            />
          </div>

          <div>
            <label for="sound_file" class="block text-sm font-medium mb-2">Sound File (optional)</label>
            <input
              id="sound_file"
              type="text"
              bind:value={newJob.sound_file}
              class="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
              placeholder="/System/Library/Sounds/Glass.aiff"
            />
          </div>

          <div>
            <label for="on_success_cmd" class="block text-sm font-medium mb-2">On Success Command (optional)</label>
            <input
              id="on_success_cmd"
              type="text"
              bind:value={newJob.on_success_cmd}
              class="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 font-mono text-sm"
              placeholder="echo 'Job completed!'"
            />
          </div>

          <div class="flex items-center gap-3">
            <input
              id="disable_macos_sleep_prevention"
              type="checkbox"
              bind:checked={newJob.disable_macos_sleep_prevention}
              class="w-4 h-4 rounded border-gray-600 bg-gray-700 text-blue-500 focus:ring-blue-500"
            />
            <label for="disable_macos_sleep_prevention" class="text-sm font-medium">
              Disable macOS sleep prevention
              <span class="text-gray-400 font-normal">(skip caffeinate wrap and pmset wake events)</span>
            </label>
          </div>

          <div class="flex gap-3 pt-4">
            <button
              type="submit"
              class="flex-1 px-6 py-2 bg-blue-600 hover:bg-blue-700 rounded-md font-medium transition-colors"
              disabled={loading}
            >
              {loading ? (editingJob ? 'Updating...' : 'Creating...') : (editingJob ? 'Update Job' : 'Create Job')}
            </button>
            <button
              type="button"
              onclick={closeModal}
              class="px-6 py-2 bg-gray-700 hover:bg-gray-600 rounded-md font-medium transition-colors"
            >
              Cancel
            </button>
          </div>
        </form>
      </div>
    </div>
  {/if}

  <!-- History Modal -->
  {#if showHistoryModal}
    <div class="fixed inset-0 bg-black/50 flex items-center justify-center p-4 z-50" onclick={closeHistoryModal}>
      <div
        class={`bg-gray-800 rounded-lg p-6 w-full shadow-xl flex flex-col ${historyModalMaximized ? 'h-full max-h-full' : 'max-w-4xl max-h-[80vh] overflow-y-auto'}`}
        onclick={(e) => e.stopPropagation()}
      >
        <div class="flex justify-between items-center mb-6">
          <h2 class="text-2xl font-semibold">Execution History: {selectedJobName}</h2>
          <div class="flex gap-2 items-center">
            {#if selectedJobHistory.length > 0}
              <button
                onclick={handleClearAllHistory}
                class="px-4 py-2 bg-red-600 hover:bg-red-700 rounded-md text-sm font-medium transition-colors"
              >
                Clear All History
              </button>
            {/if}
            <button
              onclick={() => historyModalMaximized = !historyModalMaximized}
              class="text-gray-400 hover:text-white text-xl px-2"
              title={historyModalMaximized ? 'Restore history window' : 'Maximize history window'}
              aria-label={historyModalMaximized ? 'Restore history window' : 'Maximize history window'}
            >
              {historyModalMaximized ? '↙' : '⛶'}
            </button>
            <button
              onclick={closeHistoryModal}
              class="text-gray-400 hover:text-white text-2xl px-2"
              title="Close history"
              aria-label="Close history"
            >
              ✕
            </button>
          </div>
        </div>

        <div class={historyModalMaximized ? 'flex-1 min-h-0 overflow-y-auto' : ''}>
          {#if selectedJobHistory.length === 0}
            <p class="text-gray-400 text-center py-8">No execution history yet.</p>
          {:else}
            <div class="space-y-4">
              {#each selectedJobHistory as entry (entry.id)}
                <div class="bg-gray-700 rounded-lg p-4">
                  <div class="flex justify-between items-start mb-3">
                    <div class="flex gap-4 text-sm">
                      <span class="text-gray-400">
                        {formatTimestamp(entry.timestamp)}
                      </span>
                      <span class={`px-2 py-0.5 rounded text-xs ${
                        entry.exit_code === 0
                          ? 'bg-green-900/30 text-green-400 border border-green-700'
                          : 'bg-red-900/30 text-red-400 border border-red-700'
                      }`}>
                        Exit Code: {entry.exit_code}
                      </span>
                      <span class="text-gray-400">
                        Duration: {formatDuration(entry.duration_ms)}
                      </span>
                    </div>
                    <div class="flex gap-2">
                      <button
                        onclick={() => handleCopyHistory(entry)}
                        class="px-3 py-1 bg-blue-600/50 hover:bg-blue-600 rounded text-xs font-medium transition-colors"
                        title="Copy output"
                      >
                        {copiedHistoryId === String(entry.id) ? 'Copied' : 'Copy'}
                      </button>
                      <button
                        onclick={() => handleDeleteHistory(entry.id)}
                        class="px-3 py-1 bg-red-600/50 hover:bg-red-600 rounded text-xs font-medium transition-colors"
                        title="Delete this entry"
                      >
                        Delete
                      </button>
                    </div>
                  </div>
                  {#if entry.output}
                    <div
                      class="history-markdown bg-gray-900 rounded p-3 text-sm overflow-x-auto text-gray-300"
                      onclick={handleHistoryOutputClick}
                    >
                      {@html renderMarkdown(entry.output)}
                    </div>
                  {:else}
                    <p class="text-gray-500 text-sm italic">No output</p>
                  {/if}
                </div>
              {/each}
            </div>

            {#if hasMoreHistory}
              <div class="flex justify-center pt-5">
                <button
                  onclick={() => loadHistoryPage(true)}
                  disabled={loadingMoreHistory}
                  class="px-4 py-2 bg-gray-600 hover:bg-gray-500 disabled:opacity-50 disabled:cursor-not-allowed rounded-md text-sm font-medium transition-colors"
                >
                  {loadingMoreHistory ? 'Loading...' : 'Load more'}
                </button>
              </div>
            {/if}
          {/if}
        </div>
      </div>
    </div>
  {/if}

  <!-- Custom Confirmation Dialog -->
  {#if showConfirmDialog}
    <div class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50" onclick={handleConfirmNo}>
      <div class="bg-gray-800 rounded-lg shadow-xl max-w-md w-full mx-4 p-6" onclick={(e) => e.stopPropagation()}>
        <h3 class="text-xl font-semibold mb-4">Confirm Action</h3>
        <p class="text-gray-300 mb-6">{confirmMessage}</p>
        <div class="flex justify-end gap-3">
          <button
            onclick={handleConfirmNo}
            class="px-4 py-2 bg-gray-700 hover:bg-gray-600 rounded-md font-medium transition-colors"
          >
            Cancel
          </button>
          <button
            onclick={handleConfirmYes}
            class="px-4 py-2 bg-red-600 hover:bg-red-700 rounded-md font-medium transition-colors"
          >
            Confirm
          </button>
        </div>
      </div>
    </div>
  {/if}
  
  <!-- Defaults Modal -->
  <DefaultsModal bind:show={showDefaultsModal} onSuccess={handleDefaultsSaved} />

  {/if}
</main>
