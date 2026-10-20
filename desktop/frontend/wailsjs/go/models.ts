export namespace library {
	
	export class CGEvent {
	    id: string;
	    index: number;
	    title: string;
	    thumb: string;
	    script: string;
	    label: string;
	    pages: string[];
	
	    static createFrom(source: any = {}) {
	        return new CGEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.index = source["index"];
	        this.title = source["title"];
	        this.thumb = source["thumb"];
	        this.script = source["script"];
	        this.label = source["label"];
	        this.pages = source["pages"];
	    }
	}
	export class ChapterEvent {
	    id: string;
	    index: number;
	    title: string;
	    script: string;
	    scene: string;
	    label_pos: number;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new ChapterEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.index = source["index"];
	        this.title = source["title"];
	        this.script = source["script"];
	        this.scene = source["scene"];
	        this.label_pos = source["label_pos"];
	        this.name = source["name"];
	    }
	}
	export class SceneEvent {
	    id: string;
	    title: string;
	    script: string;
	
	    static createFrom(source: any = {}) {
	        return new SceneEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.script = source["script"];
	    }
	}
	export class Library {
	    root: string;
	    game?: string;
	    generated_at: string;
	    cgs: CGEvent[];
	    scenes: SceneEvent[];
	    chapters?: ChapterEvent[];
	
	    static createFrom(source: any = {}) {
	        return new Library(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.game = source["game"];
	        this.generated_at = source["generated_at"];
	        this.cgs = this.convertValues(source["cgs"], CGEvent);
	        this.scenes = this.convertValues(source["scenes"], SceneEvent);
	        this.chapters = this.convertValues(source["chapters"], ChapterEvent);
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
	
	export class Conf {
	    root: string;
	    library: string;
	    auto: boolean;
	    conf_missing: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Conf(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.library = source["library"];
	        this.auto = source["auto"];
	        this.conf_missing = source["conf_missing"];
	    }
	}

}

