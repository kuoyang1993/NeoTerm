export namespace main {
	
	export class ConnConfig {
	    name: string;
	    ip: string;
	    port: string;
	    user: string;
	    pwd: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.ip = source["ip"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.pwd = source["pwd"];
	    }
	}

}

