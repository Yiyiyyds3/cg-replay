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
	    cont_script?: string;
	    cont_label_pos?: number;
	
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
	        this.cont_script = source["cont_script"];
	        this.cont_label_pos = source["cont_label_pos"];
	    }
	}
	export class SceneEvent {
	    id: string;
	    title: string;
	    script: string;
	    label?: string;
	
	    static createFrom(source: any = {}) {
	        return new SceneEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.script = source["script"];
	        this.label = source["label"];
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
	export class CatalogEventDTO {
	    game_id: string;
	    game_title: string;
	    game_root: string;
	    kind: string;
	    id: string;
	    title: string;
	    ord: number;
	    pages: number;
	    has_cover: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CatalogEventDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.game_id = source["game_id"];
	        this.game_title = source["game_title"];
	        this.game_root = source["game_root"];
	        this.kind = source["kind"];
	        this.id = source["id"];
	        this.title = source["title"];
	        this.ord = source["ord"];
	        this.pages = source["pages"];
	        this.has_cover = source["has_cover"];
	    }
	}
	export class CatalogEventsDTO {
	    events: CatalogEventDTO[];
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new CatalogEventsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.events = this.convertValues(source["events"], CatalogEventDTO);
	        this.warnings = source["warnings"];
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
	export class CatalogIndexResultDTO {
	    game_id: string;
	    events: number;
	    lines: number;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new CatalogIndexResultDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.game_id = source["game_id"];
	        this.events = source["events"];
	        this.lines = source["lines"];
	        this.warnings = source["warnings"];
	    }
	}
	export class CatalogRecentDTO {
	    game_id: string;
	    game_title: string;
	    kind: string;
	    event_id: string;
	    title: string;
	    played_at: string;
	    play_count: number;
	
	    static createFrom(source: any = {}) {
	        return new CatalogRecentDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.game_id = source["game_id"];
	        this.game_title = source["game_title"];
	        this.kind = source["kind"];
	        this.event_id = source["event_id"];
	        this.title = source["title"];
	        this.played_at = source["played_at"];
	        this.play_count = source["play_count"];
	    }
	}
	export class CatalogStatsDTO {
	    games: number;
	    events: number;
	    text_lines: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new CatalogStatsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.games = source["games"];
	        this.events = source["events"];
	        this.text_lines = source["text_lines"];
	        this.error = source["error"];
	    }
	}
	export class CatalogTextHitDTO {
	    line_id: number;
	    game_id: string;
	    game_title: string;
	    kind: string;
	    event_id: string;
	    t_ms: number;
	    speaker: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new CatalogTextHitDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.line_id = source["line_id"];
	        this.game_id = source["game_id"];
	        this.game_title = source["game_title"];
	        this.kind = source["kind"];
	        this.event_id = source["event_id"];
	        this.t_ms = source["t_ms"];
	        this.speaker = source["speaker"];
	        this.text = source["text"];
	    }
	}
	export class CatalogTextDTO {
	    hits: CatalogTextHitDTO[];
	    warnings: string[];
	    indexed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CatalogTextDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hits = this.convertValues(source["hits"], CatalogTextHitDTO);
	        this.warnings = source["warnings"];
	        this.indexed = source["indexed"];
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
	export class FavoriteDTO {
	    item_type: string;
	    game_id: string;
	    item_id: string;
	    rating: number;
	    note: string;
	    title: string;
	    game_title: string;
	    created_at: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new FavoriteDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.item_type = source["item_type"];
	        this.game_id = source["game_id"];
	        this.item_id = source["item_id"];
	        this.rating = source["rating"];
	        this.note = source["note"];
	        this.title = source["title"];
	        this.game_title = source["game_title"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
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
	export class GameDTO {
	    id: string;
	    title: string;
	    root: string;
	    library?: string;
	    engine?: string;
	    added_at: string;
	    last_played_at?: string;
	
	    static createFrom(source: any = {}) {
	        return new GameDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.root = source["root"];
	        this.library = source["library"];
	        this.engine = source["engine"];
	        this.added_at = source["added_at"];
	        this.last_played_at = source["last_played_at"];
	    }
	}
	export class GamesDTO {
	    active: string;
	    games: GameDTO[];
	
	    static createFrom(source: any = {}) {
	        return new GamesDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.active = source["active"];
	        this.games = this.convertValues(source["games"], GameDTO);
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
	    game_id: string;
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
	        this.game_id = source["game_id"];
	        this.stage_w = source["stage_w"];
	        this.stage_h = source["stage_h"];
	        this.op_count = source["op_count"];
	        this.text_count = source["text_count"];
	        this.duration_ms = source["duration_ms"];
	    }
	}
	export class SpeakerDTO {
	    game_id: string;
	    game_title: string;
	    name: string;
	    lines: number;
	    events: number;
	
	    static createFrom(source: any = {}) {
	        return new SpeakerDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.game_id = source["game_id"];
	        this.game_title = source["game_title"];
	        this.name = source["name"];
	        this.lines = source["lines"];
	        this.events = source["events"];
	    }
	}
	export class UnpackImportDTO {
	    dest: string;
	    engine_id: string;
	    events: number;
	    files: number;
	    bytes: number;
	    missing: string[];
	    encrypted: string[];
	    warnings: string[];
	    game_id: string;
	    connected: boolean;
	    reason?: string;
	    hint?: string;
	    library?: library.Library;
	
	    static createFrom(source: any = {}) {
	        return new UnpackImportDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dest = source["dest"];
	        this.engine_id = source["engine_id"];
	        this.events = source["events"];
	        this.files = source["files"];
	        this.bytes = source["bytes"];
	        this.missing = source["missing"];
	        this.encrypted = source["encrypted"];
	        this.warnings = source["warnings"];
	        this.game_id = source["game_id"];
	        this.connected = source["connected"];
	        this.reason = source["reason"];
	        this.hint = source["hint"];
	        this.library = this.convertValues(source["library"], library.Library);
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
	export class UnpackPlanDTO {
	    engine_id: string;
	    kind: string;
	    archives: string[];
	    scenario_files: number;
	    labels: number;
	    refs: number;
	    encrypted: number;
	    notes: string[];
	    unsupported: boolean;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new UnpackPlanDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.engine_id = source["engine_id"];
	        this.kind = source["kind"];
	        this.archives = source["archives"];
	        this.scenario_files = source["scenario_files"];
	        this.labels = source["labels"];
	        this.refs = source["refs"];
	        this.encrypted = source["encrypted"];
	        this.notes = source["notes"];
	        this.unsupported = source["unsupported"];
	        this.reason = source["reason"];
	    }
	}

}

