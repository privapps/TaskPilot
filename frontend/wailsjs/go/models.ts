export namespace models {
	
	export class Defaults {
	    working_directory: string;
	    sound_file: string;
	    on_success_cmd: string;
	    api_port: number;
	
	    static createFrom(source: any = {}) {
	        return new Defaults(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.working_directory = source["working_directory"];
	        this.sound_file = source["sound_file"];
	        this.on_success_cmd = source["on_success_cmd"];
	        this.api_port = source["api_port"];
	    }
	}
	export class History {
	    id: string;
	    job_id: string;
	    output: string;
	    exit_code: number;
	    timestamp: number;
	    duration_ms: number;
	    scheduled_at?: number;
	    trigger_type?: string;
	
	    static createFrom(source: any = {}) {
	        return new History(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.job_id = source["job_id"];
	        this.output = source["output"];
	        this.exit_code = source["exit_code"];
	        this.timestamp = source["timestamp"];
	        this.duration_ms = source["duration_ms"];
	        this.scheduled_at = source["scheduled_at"];
	        this.trigger_type = source["trigger_type"];
	    }
	}
	export class Job {
	    id: string;
	    name: string;
	    command: string;
	    directory: string;
	    schedule: string;
	    sound_file: string;
	    on_success_cmd: string;
	    last_result: string;
	    status: string;
	    schedule_type: string;
	    paused: boolean;
	    run_at?: number;
	    delay_minutes?: number;
	    last_run_at?: number;
	    next_run_at?: number;
	    last_scheduled_at?: number;
	    disable_macos_sleep_prevention?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Job(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.command = source["command"];
	        this.directory = source["directory"];
	        this.schedule = source["schedule"];
	        this.sound_file = source["sound_file"];
	        this.on_success_cmd = source["on_success_cmd"];
	        this.last_result = source["last_result"];
	        this.status = source["status"];
	        this.schedule_type = source["schedule_type"];
	        this.paused = source["paused"];
	        this.run_at = source["run_at"];
	        this.delay_minutes = source["delay_minutes"];
	        this.last_run_at = source["last_run_at"];
	        this.next_run_at = source["next_run_at"];
	        this.last_scheduled_at = source["last_scheduled_at"];
	        this.disable_macos_sleep_prevention = source["disable_macos_sleep_prevention"];
	    }
	}

}

export namespace services {
	
	export class LaunchAgentStatus {
	    label: string;
	    plist_path: string;
	    installed: boolean;
	    loaded: boolean;
	    port: number;
	
	    static createFrom(source: any = {}) {
	        return new LaunchAgentStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.plist_path = source["plist_path"];
	        this.installed = source["installed"];
	        this.loaded = source["loaded"];
	        this.port = source["port"];
	    }
	}
	export class Scheduler {
	
	
	    static createFrom(source: any = {}) {
	        return new Scheduler(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}

