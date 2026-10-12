export namespace app {

	export class LocalityView {
	    exit_ip: string;
	    country_code: string;
	    city?: string;
	    timezone: string;
	    locale: string;

	    static createFrom(source: any = {}) {
	        return new LocalityView(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exit_ip = source["exit_ip"];
	        this.country_code = source["country_code"];
	        this.city = source["city"];
	        this.timezone = source["timezone"];
	        this.locale = source["locale"];
	    }
	}
	export class NetworkView {
	    mode: string;
	    scheme?: string;
	    host?: string;
	    port?: number;
	    username?: string;
	    has_password: boolean;
	    status: string;
	    locality?: LocalityView;

	    static createFrom(source: any = {}) {
	        return new NetworkView(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.scheme = source["scheme"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.has_password = source["has_password"];
	        this.status = source["status"];
	        this.locality = this.convertValues(source["locality"], LocalityView);
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
	export class ProfileView {
	    id: string;
	    name: string;
	    browser: string;
	    network: string;
	    status: string;
	    launch_ready: boolean;
	    launch_blocked_reason?: string;

	    static createFrom(source: any = {}) {
	        return new ProfileView(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.browser = source["browser"];
	        this.network = source["network"];
	        this.status = source["status"];
	        this.launch_ready = source["launch_ready"];
	        this.launch_blocked_reason = source["launch_blocked_reason"];
	    }
	}
	export class SetProxyInput {
	    scheme: string;
	    host: string;
	    port: number;
	    username: string;
	    password: string;

	    static createFrom(source: any = {}) {
	        return new SetProxyInput(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scheme = source["scheme"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.password = source["password"];
	    }
	}

}

export namespace enginebootstrap {

	export class Status {
	    enabled: boolean;
	    ready: boolean;
	    phase: string;
	    received: number;
	    total: number;

	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.ready = source["ready"];
	        this.phase = source["phase"];
	        this.received = source["received"];
	        this.total = source["total"];
	    }
	}

}
