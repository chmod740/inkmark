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
	export class Document {
	    path: string;
	    name: string;
	    content: string;
	    welcome: boolean;
	    builtIn?: string;

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

}
