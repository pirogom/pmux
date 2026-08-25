export namespace config {
	
	export class SavedLayoutNode {
	    id?: string;
	    direction?: string;
	    ratio?: number;
	    children?: SavedLayoutNode[];
	    command?: string;
	    args?: string[];
	    workDir?: string;
	
	    static createFrom(source: any = {}) {
	        return new SavedLayoutNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.direction = source["direction"];
	        this.ratio = source["ratio"];
	        this.children = this.convertValues(source["children"], SavedLayoutNode);
	        this.command = source["command"];
	        this.args = source["args"];
	        this.workDir = source["workDir"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Profile {
	    id: string;
	    name: string;
	    command: string;
	    args: string[];
	    workDir: string;
	    env: string[];
	    isPreset: boolean;
	    savedLayout?: SavedLayoutNode;
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.command = source["command"];
	        this.args = source["args"];
	        this.workDir = source["workDir"];
	        this.env = source["env"];
	        this.isPreset = source["isPreset"];
	        this.savedLayout = this.convertValues(source["savedLayout"], SavedLayoutNode);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Config {
	    defaultProfileId: string;
	    profiles: Profile[];
	    serverPort: number;
	    theme: string;
	    gitPollInterval: number;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.defaultProfileId = source["defaultProfileId"];
	        this.profiles = this.convertValues(source["profiles"], Profile);
	        this.serverPort = source["serverPort"];
	        this.theme = source["theme"];
	        this.gitPollInterval = source["gitPollInterval"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	

}

export namespace git {
	
	export class GitBranch {
	    name: string;
	    current: boolean;
	    upstream?: string;
	
	    static createFrom(source: any = {}) {
	        return new GitBranch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.current = source["current"];
	        this.upstream = source["upstream"];
	    }
	}
	export class GitChange {
	    status: string;
	    staged: boolean;
	    unstaged: boolean;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new GitChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.staged = source["staged"];
	        this.unstaged = source["unstaged"];
	        this.path = source["path"];
	    }
	}
	export class GitCommit {
	    hash: string;
	    shortHash: string;
	    author: string;
	    email: string;
	    date: string;
	    message: string;
	    subject: string;
	    refs?: string[];
	    isHead: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GitCommit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.shortHash = source["shortHash"];
	        this.author = source["author"];
	        this.email = source["email"];
	        this.date = source["date"];
	        this.message = source["message"];
	        this.subject = source["subject"];
	        this.refs = source["refs"];
	        this.isHead = source["isHead"];
	    }
	}
	export class GitCommitDetail {
	    hash: string;
	    shortHash: string;
	    author: string;
	    email: string;
	    date: string;
	    subject: string;
	    message: string;
	    parent?: string;
	    diff: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new GitCommitDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.shortHash = source["shortHash"];
	        this.author = source["author"];
	        this.email = source["email"];
	        this.date = source["date"];
	        this.subject = source["subject"];
	        this.message = source["message"];
	        this.parent = source["parent"];
	        this.diff = source["diff"];
	        this.error = source["error"];
	    }
	}
	export class GitDiffResult {
	    path: string;
	    staged?: string;
	    unstaged?: string;
	    binary: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new GitDiffResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.staged = source["staged"];
	        this.unstaged = source["unstaged"];
	        this.binary = source["binary"];
	        this.error = source["error"];
	    }
	}
	export class GitOpResult {
	    success: boolean;
	    output: string;
	    error?: string;
	    auth?: string;
	
	    static createFrom(source: any = {}) {
	        return new GitOpResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.output = source["output"];
	        this.error = source["error"];
	        this.auth = source["auth"];
	    }
	}
	export class GitRemote {
	    name: string;
	    urls: string[];
	
	    static createFrom(source: any = {}) {
	        return new GitRemote(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.urls = source["urls"];
	    }
	}
	export class GitStatusResult {
	    isGitRepo: boolean;
	    branch: string;
	    root?: string;
	    changes: GitChange[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new GitStatusResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.isGitRepo = source["isGitRepo"];
	        this.branch = source["branch"];
	        this.root = source["root"];
	        this.changes = this.convertValues(source["changes"], GitChange);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace main {
	
	export class CreateSessionReq {
	    profileId: string;
	    name: string;
	    command: string;
	    args: string[];
	    workDir: string;
	    cols: number;
	    rows: number;
	
	    static createFrom(source: any = {}) {
	        return new CreateSessionReq(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.name = source["name"];
	        this.command = source["command"];
	        this.args = source["args"];
	        this.workDir = source["workDir"];
	        this.cols = source["cols"];
	        this.rows = source["rows"];
	    }
	}
	export class SplitPaneReq {
	    sessionId: string;
	    parentPaneId: string;
	    direction: string;
	    command: string;
	    args: string[];
	    workDir: string;
	    cols: number;
	    rows: number;
	
	    static createFrom(source: any = {}) {
	        return new SplitPaneReq(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.parentPaneId = source["parentPaneId"];
	        this.direction = source["direction"];
	        this.command = source["command"];
	        this.args = source["args"];
	        this.workDir = source["workDir"];
	        this.cols = source["cols"];
	        this.rows = source["rows"];
	    }
	}

}

export namespace ssh {
	
	export class Address {
	    id: string;
	    name: string;
	    description: string;
	    host: string;
	    user: string;
	
	    static createFrom(source: any = {}) {
	        return new Address(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.host = source["host"];
	        this.user = source["user"];
	    }
	}
	export class Config {
	    clientPath: string;
	    addresses: Address[];
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.clientPath = source["clientPath"];
	        this.addresses = this.convertValues(source["addresses"], Address);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

