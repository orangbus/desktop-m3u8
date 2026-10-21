export namespace core {
	
	export class Settings {
	    maxConcurrentTasks: number;
	    concurrencyPerTask: number;
	    savePath: string;
	    apiPort: number;
	    apiBaseUrl: string;
	    apiKey: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.maxConcurrentTasks = source["maxConcurrentTasks"];
	        this.concurrencyPerTask = source["concurrencyPerTask"];
	        this.savePath = source["savePath"];
	        this.apiPort = source["apiPort"];
	        this.apiBaseUrl = source["apiBaseUrl"];
	        this.apiKey = source["apiKey"];
	    }
	}
	export class Task {
	    id: string;
	    url: string;
	    filename: string;
	    headers: Record<string, string>;
	    status: string;
	    progress: number;
	    speed: number;
	    totalBytes: number;
	    segments: number;
	    doneSegs: number;
	    error: string;
	    outputPath: string;
	    createdAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.url = source["url"];
	        this.filename = source["filename"];
	        this.headers = source["headers"];
	        this.status = source["status"];
	        this.progress = source["progress"];
	        this.speed = source["speed"];
	        this.totalBytes = source["totalBytes"];
	        this.segments = source["segments"];
	        this.doneSegs = source["doneSegs"];
	        this.error = source["error"];
	        this.outputPath = source["outputPath"];
	        this.createdAt = source["createdAt"];
	    }
	}

}

