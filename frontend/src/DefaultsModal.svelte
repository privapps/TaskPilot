<script>
  import { GetDefaults, UpdateDefaults } from '../wailsjs/go/main/App';
  
  export let show = false;
  export let onSuccess = () => {};
  
  let workingDirectory = '';
  let soundFile = '';
  let onSuccessCmd = '';
  let apiPort = 8080;
  let loading = false;
  let error = '';
  let validationWarning = '';
  let showRestartMessage = false;
  
  // Reactive statement to load defaults when modal opens
  $: if (show) {
    loadDefaults();
    showRestartMessage = false;
  }
  
  async function loadDefaults() {
    try {
      loading = true;
      error = '';
      const defaults = await GetDefaults();
      if (defaults) {
        workingDirectory = defaults.working_directory || '';
        soundFile = defaults.sound_file || '';
        onSuccessCmd = defaults.on_success_cmd || '';
        apiPort = defaults.api_port || 8080;
      }
    } catch (err) {
      error = 'Failed to load defaults: ' + err;
    } finally {
      loading = false;
    }
  }
  
  async function handleSave() {
    try {
      loading = true;
      error = '';
      validationWarning = '';
      
      // Basic validation warning for non-existent directory
      if (workingDirectory && workingDirectory.trim()) {
        // We can't actually check if the directory exists from the frontend
        // but we can show a warning if it looks suspicious
        if (!workingDirectory.startsWith('/') && !workingDirectory.match(/^[A-Z]:/)) {
          validationWarning = 'Directory path should be absolute (start with / or C:)';
        }
      }
      
      // Validate API port range
      if (apiPort < 1024 || apiPort > 65535) {
        validationWarning = 'API port must be between 1024 and 65535';
        loading = false;
        return;
      }
      
      const defaults = {
        working_directory: workingDirectory,
        sound_file: soundFile,
        on_success_cmd: onSuccessCmd,
        api_port: apiPort
      };
      
      await UpdateDefaults(defaults);
      showRestartMessage = true;
      
      // Auto-close after 2 seconds
      setTimeout(() => {
        onSuccess();
        show = false;
      }, 2000);
    } catch (err) {
      error = 'Failed to save defaults: ' + err;
    } finally {
      loading = false;
    }
  }
  
  function handleCancel() {
    show = false;
    error = '';
    validationWarning = '';
  }
  
  function handleKeyDown(event) {
    if (event.key === 'Escape') {
      handleCancel();
    }
  }
</script>

<svelte:window onkeydown={handleKeyDown} />

{#if show}
  <div class="fixed inset-0 bg-black/50 flex items-center justify-center p-4 z-50" onclick={handleCancel}>
    <div class="bg-gray-800 rounded-lg p-6 max-w-2xl w-full shadow-xl" onclick={(e) => e.stopPropagation()}>
      <h2 class="text-2xl font-semibold mb-6">Default Job Settings</h2>
      
      {#if error}
        <div class="mb-4 p-4 bg-red-900/30 border border-red-700 rounded-md text-red-300">
          {error}
        </div>
      {/if}
      
      {#if validationWarning}
        <div class="mb-4 p-4 bg-yellow-900/30 border border-yellow-700 rounded-md text-yellow-300">
          ⚠️ {validationWarning}
        </div>
      {/if}
      
      {#if showRestartMessage}
        <div class="mb-4 p-4 bg-green-900/30 border border-green-700 rounded-md text-green-300">
          ✓ Settings saved! Restart the application for API port changes to take effect.
        </div>
      {/if}
      
      <form onsubmit={(e) => { e.preventDefault(); handleSave(); }} class="space-y-4">
        <div>
          <label for="default-directory" class="block text-sm font-medium mb-2">
            Working Directory
            <span class="text-gray-400 font-normal text-xs ml-2">
              (auto-filled when creating new jobs)
            </span>
          </label>
          <input
            id="default-directory"
            type="text"
            bind:value={workingDirectory}
            class="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
            placeholder="/path/to/working/directory"
            disabled={loading}
          />
        </div>
        
        <div>
          <label for="default-sound" class="block text-sm font-medium mb-2">
            Sound File
            <span class="text-gray-400 font-normal text-xs ml-2">
              (played when job completes)
            </span>
          </label>
          <input
            id="default-sound"
            type="text"
            bind:value={soundFile}
            class="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
            placeholder="/path/to/sound.mp3"
            disabled={loading}
          />
        </div>
        
        <div>
          <label for="default-success-cmd" class="block text-sm font-medium mb-2">
            On Success Command
            <span class="text-gray-400 font-normal text-xs ml-2">
              (runs after successful job execution)
            </span>
          </label>
          <input
            id="default-success-cmd"
            type="text"
            bind:value={onSuccessCmd}
            class="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 font-mono text-sm"
            placeholder="echo 'Job completed!'"
            disabled={loading}
          />
        </div>
        
        <div>
          <label for="api-port" class="block text-sm font-medium mb-2">
            API Server Port
            <span class="text-gray-400 font-normal text-xs ml-2">
              (requires restart to take effect)
            </span>
          </label>
          <input
            id="api-port"
            type="number"
            bind:value={apiPort}
            min="1024"
            max="65535"
            class="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
            placeholder="8080"
            disabled={loading}
          />
          <p class="text-xs text-gray-400 mt-1">
            Valid range: 1024-65535. Access API documentation at http://localhost:{apiPort}/api/swagger
          </p>
        </div>
        
        <div class="flex justify-end gap-3 mt-6">
          <button
            type="button"
            onclick={handleCancel}
            class="px-6 py-2 bg-gray-700 hover:bg-gray-600 rounded-md font-medium transition-colors"
            disabled={loading}
          >
            Cancel
          </button>
          <button
            type="submit"
            class="px-6 py-2 bg-blue-600 hover:bg-blue-700 rounded-md font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            disabled={loading}
          >
            {loading ? 'Saving...' : 'Save'}
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}
