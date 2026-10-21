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
	
	export class LineDTO {
	    speaker: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new LineDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.speaker = source["speaker"];
	        this.text = source["text"];
	    }
	}
	export class rgbaDTO {
	    R: number;
	    G: number;
	    B: number;
	
	    static createFrom(source: any = {}) {
	        return new rgbaDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.R = source["R"];
	        this.G = source["G"];
	        this.B = source["B"];
	    }
	}
	export class BoxDTO {
	    layer: string;
	    x: number;
	    y: number;
	    w: number;
	    h: number;
	    has_geom: boolean;
	    visible: boolean;
	    frame_color: rgbaDTO;
	    frame_opacity: number;
	    font_size: number;
	    line_spacing: number;
	    lines: LineDTO[];
	
	    static createFrom(source: any = {}) {
	        return new BoxDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.layer = source["layer"];
	        this.x = source["x"];
	        this.y = source["y"];
	        this.w = source["w"];
	        this.h = source["h"];
	        this.has_geom = source["has_geom"];
	        this.visible = source["visible"];
	        this.frame_color = this.convertValues(source["frame_color"], rgbaDTO);
	        this.frame_opacity = source["frame_opacity"];
	        this.font_size = source["font_size"];
	        this.line_spacing = source["line_spacing"];
	        this.lines = this.convertValues(source["lines"], LineDTO);
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
	export class LayerDTO {
	    layer: string;
	    ref: string;
	    visible: boolean;
	    x: number;
	    y: number;
	    opacity: number;
	    z: number;
	    missing: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LayerDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.layer = source["layer"];
	        this.ref = source["ref"];
	        this.visible = source["visible"];
	        this.x = source["x"];
	        this.y = source["y"];
	        this.opacity = source["opacity"];
	        this.z = source["z"];
	        this.missing = source["missing"];
	    }
	}
	export class FrameDTO {
	    jpeg: string;
	    now: number;
	    dur_ms: number;
	    playing: boolean;
	    done: boolean;
	    waiting: boolean;
	    auto: boolean;
	    muted: boolean;
	    quake_x: number;
	    quake_y: number;
	    missing: string[];
	    layers: LayerDTO[];
	    texts: BoxDTO[];
	    cached: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FrameDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jpeg = source["jpeg"];
	        this.now = source["now"];
	        this.dur_ms = source["dur_ms"];
	        this.playing = source["playing"];
	        this.done = source["done"];
	        this.waiting = source["waiting"];
	        this.auto = source["auto"];
	        this.muted = source["muted"];
	        this.quake_x = source["quake_x"];
	        this.quake_y = source["quake_y"];
	        this.missing = source["missing"];
	        this.layers = this.convertValues(source["layers"], LayerDTO);
	        this.texts = this.convertValues(source["texts"], BoxDTO);
	        this.cached = source["cached"];
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
	
	
	export class SessionMeta {
	    kind: string;
	    id: string;
	    title: string;
	    stage_w: number;
	    stage_h: number;
	    op_count: number;
	    text_count: number;
	    duration_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.id = source["id"];
	        this.title = source["title"];
	        this.stage_w = source["stage_w"];
	        this.stage_h = source["stage_h"];
	        this.op_count = source["op_count"];
	        this.text_count = source["text_count"];
	        this.duration_ms = source["duration_ms"];
	    }
	}

}

