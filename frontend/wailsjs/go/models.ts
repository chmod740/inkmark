export namespace main {

	export class AppInfo {
	    version: string;
	    author: string;
	    repositoryURL: string;

	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.author = source["author"];
	        this.repositoryURL = source["repositoryURL"];
	    }
	}
	export class WorkspaceEntry {
	    name: string;
	    path: string;
	    absolutePath: string;
	    kind: string;
	    revision?: string;

	    static createFrom(source: any = {}) {
	        return new WorkspaceEntry(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.absolutePath = source["absolutePath"];
	        this.kind = source["kind"];
	        this.revision = source["revision"];
	    }
	}
	export class Workspace {
	    id: string;
	    provider: string;
	    name: string;
	    path: string;
	    entries: WorkspaceEntry[];
	    truncated: boolean;

	    static createFrom(source: any = {}) {
	        return new Workspace(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.provider = source["provider"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.entries = this.convertValues(source["entries"], WorkspaceEntry);
	        this.truncated = source["truncated"];
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
	export class Document {
	    path: string;
	    name: string;
	    content: string;
	    welcome: boolean;
	    builtIn?: string;
	    activationId?: string;
	    storageKind?: string;
	    displayLocation?: string;
	    workspaceId?: string;
	    workspacePath?: string;
	    localDocumentId?: string;
	    remoteDocumentId?: string;
	    etag?: string;
	    workspace?: Workspace;

	    static createFrom(source: any = {}) {
	        return new Document(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.content = source["content"];
	        this.welcome = source["welcome"];
	        this.builtIn = source["builtIn"];
	        this.activationId = source["activationId"];
	        this.storageKind = source["storageKind"];
	        this.displayLocation = source["displayLocation"];
	        this.workspaceId = source["workspaceId"];
	        this.workspacePath = source["workspacePath"];
	        this.localDocumentId = source["localDocumentId"];
	        this.remoteDocumentId = source["remoteDocumentId"];
	        this.etag = source["etag"];
	        this.workspace = this.convertValues(source["workspace"], Workspace);
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
	export class ImageAsset {
	    markdownURL: string;
	    name: string;
	    mimeType: string;
	    size: number;
	    width: number;
	    height: number;
	    sha256: string;

	    static createFrom(source: any = {}) {
	        return new ImageAsset(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.markdownURL = source["markdownURL"];
	        this.name = source["name"];
	        this.mimeType = source["mimeType"];
	        this.size = source["size"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.sha256 = source["sha256"];
	    }
	}
	export class ImageAssetData {
	    name: string;
	    mimeType: string;
	    dataBase64: string;
	    size: number;
	    width: number;
	    height: number;
	    sha256: string;

	    static createFrom(source: any = {}) {
	        return new ImageAssetData(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.mimeType = source["mimeType"];
	        this.dataBase64 = source["dataBase64"];
	        this.size = source["size"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.sha256 = source["sha256"];
	    }
	}
	export class LanguageState {
	    mode: string;
	    locale: string;

	    static createFrom(source: any = {}) {
	        return new LanguageState(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.locale = source["locale"];
	    }
	}
	export class RecentWebDAVConnection {
	    endpoint: string;
	    name: string;
	    savedConnectionId?: string;
	    hasSavedCredentials: boolean;

	    static createFrom(source: any = {}) {
	        return new RecentWebDAVConnection(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpoint = source["endpoint"];
	        this.name = source["name"];
	        this.savedConnectionId = source["savedConnectionId"];
	        this.hasSavedCredentials = source["hasSavedCredentials"];
	    }
	}
	export class SaveResult {
	    path: string;
	    name: string;

	    static createFrom(source: any = {}) {
	        return new SaveResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	    }
	}
	export class SavedWebDAVConnection {
	    id: string;
	    name: string;
	    endpoint: string;
	    username: string;
	    hasCredentials: boolean;
	    credentialsAvailable: boolean;
	    usernamePresent: boolean;

	    static createFrom(source: any = {}) {
	        return new SavedWebDAVConnection(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.endpoint = source["endpoint"];
	        this.username = source["username"];
	        this.hasCredentials = source["hasCredentials"];
	        this.credentialsAvailable = source["credentialsAvailable"];
	        this.usernamePresent = source["usernamePresent"];
	    }
	}
	export class UpdateDownload {
	    sessionID: string;
	    assetName: string;
	    version: string;
	    bytesDownloaded: number;
	    totalBytes: number;
	    progress: number;
	    ready: boolean;

	    static createFrom(source: any = {}) {
	        return new UpdateDownload(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionID = source["sessionID"];
	        this.assetName = source["assetName"];
	        this.version = source["version"];
	        this.bytesDownloaded = source["bytesDownloaded"];
	        this.totalBytes = source["totalBytes"];
	        this.progress = source["progress"];
	        this.ready = source["ready"];
	    }
	}
	export class UpdateInfo {
	    currentVersion: string;
	    latestVersion: string;
	    updateAvailable: boolean;
	    releaseURL: string;
	    downloadURL: string;
	    publishedAt: string;
	    assetName: string;
	    assetSize: number;
	    installable: boolean;
	    checksumAvailable: boolean;
	    installerKind: string;

	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.currentVersion = source["currentVersion"];
	        this.latestVersion = source["latestVersion"];
	        this.updateAvailable = source["updateAvailable"];
	        this.releaseURL = source["releaseURL"];
	        this.downloadURL = source["downloadURL"];
	        this.publishedAt = source["publishedAt"];
	        this.assetName = source["assetName"];
	        this.assetSize = source["assetSize"];
	        this.installable = source["installable"];
	        this.checksumAvailable = source["checksumAvailable"];
	        this.installerKind = source["installerKind"];
	    }
	}
	export class WebDAVConfig {
	    endpoint: string;
	    username: string;
	    password: string;

	    static createFrom(source: any = {}) {
	        return new WebDAVConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpoint = source["endpoint"];
	        this.username = source["username"];
	        this.password = source["password"];
	    }
	}
	export class WebDAVConnectionInput {
	    id: string;
	    name: string;
	    endpoint: string;
	    username: string;
	    password: string;
	    replaceCredentials: boolean;
	    removeCredentials: boolean;

	    static createFrom(source: any = {}) {
	        return new WebDAVConnectionInput(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.endpoint = source["endpoint"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.replaceCredentials = source["replaceCredentials"];
	        this.removeCredentials = source["removeCredentials"];
	    }
	}
	export class WebDAVMutationPreparation {
	    mutationId: string;
	    entry: WorkspaceEntry;
	    expiresAt: string;

	    static createFrom(source: any = {}) {
	        return new WebDAVMutationPreparation(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mutationId = source["mutationId"];
	        this.entry = this.convertValues(source["entry"], WorkspaceEntry);
	        this.expiresAt = source["expiresAt"];
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
	export class WebDAVSaveResult {
	    path: string;
	    name: string;
	    etag: string;
	    conflict: boolean;

	    static createFrom(source: any = {}) {
	        return new WebDAVSaveResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.etag = source["etag"];
	        this.conflict = source["conflict"];
	    }
	}

	export class WorkspaceDirectory {
	    path: string;
	    entries: WorkspaceEntry[];
	    truncated: boolean;

	    static createFrom(source: any = {}) {
	        return new WorkspaceDirectory(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.entries = this.convertValues(source["entries"], WorkspaceEntry);
	        this.truncated = source["truncated"];
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
