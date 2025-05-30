import {
    AfterViewInit,
    ChangeDetectionStrategy,
    ChangeDetectorRef,
    Component,
    ComponentRef, ElementRef,
    EventEmitter,
    HostListener,
    Input,
    OnDestroy,
    Output,
    ViewChild,
    ViewContainerRef
} from '@angular/core';
import { GraphStageNodeComponent } from './node/stage-node.component';
import { GraphForkJoinNodeComponent } from './node/fork-join-node.components';
import { GraphJobNodeComponent } from './node/job-node.component';
import { GraphNode, GraphNodeType, NavigationGraph } from './graph.model';
import { GraphDirection, NodeMouseEvent, WorkflowV2Graph } from './graph.lib';
import { load, LoadOptions } from 'js-yaml';
import { V2Workflow, V2WorkflowRun, V2WorkflowRunJob, V2WorkflowRunJobStatusIsActive } from './v2.workflow.run.model';
import { GraphMatrixNodeComponent } from './node/matrix-node.component';
import { GraphNodeAction } from './node/model';

export type WorkflowV2JobsGraphOrNodeOrMatrixComponent = GraphStageNodeComponent | GraphForkJoinNodeComponent | GraphJobNodeComponent | GraphMatrixNodeComponent;

@Component({
    selector: 'app-graph',
    templateUrl: './graph.html',
    styleUrls: ['./graph.scss'],
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class GraphComponent implements AfterViewInit, OnDestroy {
    static maxScale = 15;
    static minScale = 1 / 5;

    @ViewChild('svgGraph', { read: ViewContainerRef }) svgContainer: ViewContainerRef;

    @Input() set workflow(data: any) {
        // Parse the workflow
        let workflow: V2Workflow;
        try {
            workflow = load(data && data !== '' ? data : '{}', <LoadOptions>{
                onWarning: (e) => { }
            });
        } catch (e) {
            console.error("Invalid workflow:", data, e)
        }

        this.hasStages = !!workflow && !!workflow.stages;

        this.nodes = [];
        if (this.hasStages) {
            this.nodes.push(...Object.keys(workflow.stages).map(k => <GraphNode>{
                type: GraphNodeType.Stage,
                name: k,
                depends_on: workflow.stages[k]?.needs,
                sub_graph: []
            }));
        }

        if (workflow && workflow.jobs) {
            Object.keys(workflow.jobs).forEach(jobName => {
                const jobSpec = workflow.jobs[jobName];

                let node = <GraphNode>{
                    type: GraphNodeType.Job,
                    name: jobName,
                    depends_on: jobSpec?.needs,
                    job: jobSpec
                };
                if (jobSpec.gate) {
                    node.gate = workflow.gates[jobSpec.gate];
                }

                if (jobSpec?.stage) {
                    for (let i = 0; i < this.nodes.length; i++) {
                        if (this.nodes[i].name === jobSpec.stage && this.nodes[i].type === GraphNodeType.Stage) {
                            this.nodes[i].sub_graph.push(node);
                            break;
                        }
                    }
                } else {
                    this.nodes.push(node);
                }
            });
        }

        this.initRunJobs();
        this.initGate();

        this.hooks = [];
        this.selectedHook = '';
        if (workflow && workflow.on) {
            this.hooksOn = workflow.on;
            this.initHooks();
        }

        this.changeDisplay();
        this._cd.markForCheck();
    }

    _runJobs: Array<V2WorkflowRunJob> = [];

    @Input() set runJobs(data: Array<V2WorkflowRunJob>) {
        this._runJobs = data ?? [];
        if (!this.svgContainer) {
            return;
        }
        this.initRunJobs();
        this.initGraph();
    }

    _workflowRun: V2WorkflowRun
    @Input() set workflowRun(data: V2WorkflowRun) {
        this._workflowRun = data;
        this.initHooks();
        this.initGate();
    }

    @Input() navigationDisabled: boolean = false;

    @Output() onSelectJob = new EventEmitter<string>();
    @Output() onSelectJobGate = new EventEmitter<string>();
    @Output() onConfirmJobGate = new EventEmitter<string>();
    @Output() onSelectJobRun = new EventEmitter<string>();
    @Output() onSelectJobRunRestart = new EventEmitter<string>();
    @Output() onSelectJobRunStop = new EventEmitter<string>();
    @Output() onSelectHook = new EventEmitter<string>();

    nodes: Array<GraphNode> = [];
    hooks: Array<any> = [];
    selectedHook: string;
    hooksOn: any;
    selectedNodeNavigationKey: string;
    navigationGraph: NavigationGraph;
    direction: GraphDirection = GraphDirection.HORIZONTAL;
    ready: boolean;
    hasStages = false;
    graph: WorkflowV2Graph<WorkflowV2JobsGraphOrNodeOrMatrixComponent>;

    constructor(
        private _cd: ChangeDetectorRef,
        private host: ElementRef
    ) {
        const observer = new ResizeObserver(entries => {
            this.onResize();
        });
        observer.observe(this.host.nativeElement);
    }

    initHooks(): void {
        this.hooks = [];
        this.selectedHook = '';
        if (this.hooksOn) {
            if (Object.prototype.toString.call(this.hooksOn) === '[object Array]') {
                this.hooks = this.hooksOn;
            } else {
                this.hooks = Object.keys(this.hooksOn);
            }
        }
        this.selectedHook = this._workflowRun?.event?.event_name;
    }

    ngOnDestroy(): void { } // Should be set to use @AutoUnsubscribe with AOT

    ngAfterViewInit(): void {
        this.ready = true;
        this._cd.detectChanges();
        this.changeDisplay();
    }

    @HostListener('window:keydown', ['$event'])
    handleKeyDown(event: KeyboardEvent) {
        if (!this.navigationGraph || this.navigationDisabled) { return; }
        let newSelected: string = null;
        switch (event.key) {
            case 'ArrowDown':
                newSelected = this.direction === GraphDirection.HORIZONTAL ? this.navigationGraph.getSideNext(this.selectedNodeNavigationKey) : this.navigationGraph.getNext(this.selectedNodeNavigationKey);
                break;
            case 'ArrowUp':
                newSelected = this.direction === GraphDirection.HORIZONTAL ? this.navigationGraph.getSidePrevious(this.selectedNodeNavigationKey) : this.navigationGraph.getPrevious(this.selectedNodeNavigationKey);
                break;
            case 'ArrowLeft':
                newSelected = this.direction === GraphDirection.HORIZONTAL ? this.navigationGraph.getPrevious(this.selectedNodeNavigationKey) : this.navigationGraph.getSidePrevious(this.selectedNodeNavigationKey);
                break;
            case 'ArrowRight':
                newSelected = this.direction === GraphDirection.HORIZONTAL ? this.navigationGraph.getNext(this.selectedNodeNavigationKey) : this.navigationGraph.getSideNext(this.selectedNodeNavigationKey);
                break;
            case 'Enter':
                if (this.selectedNodeNavigationKey) {
                    this.graph.activateNode(this.selectedNodeNavigationKey);
                }
                return;
            default:
                return;
        }
        if (newSelected) {
            this.selectedNodeNavigationKey = newSelected;
            this.graph.selectNode(this.selectedNodeNavigationKey);
            this.graph.centerNode(this.selectedNodeNavigationKey, true);
        }
    }

    unSelect() {
        this.graph.selectNode(null);
        this.graph.centeredNode = null;
    }

    onResize() {
        this.resize();
    }

    changeDisplay(): void {
        if (!this.ready) {
            return;
        }
        this.initGraph();
    }

    initGate(): void {
        if (!this._workflowRun || !this.nodes) {
            return;
        }
        if (!this._workflowRun.job_events || this._workflowRun.job_events.length === 0) {
            return;
        }
        this._workflowRun.job_events.forEach(e => {
            this.nodes.forEach(n => {
                if (n.sub_graph) {
                    n.sub_graph.forEach(sub => {
                        if (sub.name === e.job_id) {
                            sub.event = e;
                        }
                    });
                } else {
                    if (n.name === e.job_id) {
                        n.event = e;
                    }
                };
            });
        });
    }

    initRunJobs(): void {
        // Clean run job data on nodes
        this.nodes.forEach(n => {
            if (n.sub_graph) {
                n.sub_graph.forEach(sub => {
                    delete sub.run;
                    delete sub.runs;
                });
            } else {
                delete n.run;
                delete n.runs;
            }
        });

        // Add run job data on nodes
        this._runJobs.forEach(j => {
            let isMatrix = false;
            if (j.matrix) {
                isMatrix = true
            }
            this.nodes.forEach(n => {
                if (n.sub_graph) {
                    n.sub_graph.forEach(sub => {
                        if (sub.name === j.job_id) {
                            if (isMatrix) {
                                sub.job.strategy.matrix = j.job.strategy.matrix;
                                sub.type = GraphNodeType.Matrix;
                                sub.runs = (sub.runs ?? []).concat(j);
                            } else {
                                sub.run = j;
                            }
                        }
                    });
                } else {
                    if (n.name === j.job_id) {
                        if (isMatrix) {
                            n.job.strategy.matrix = j.job.strategy.matrix;
                            n.type = GraphNodeType.Matrix;
                            n.runs = (n.runs ?? []).concat(j);
                        } else {
                            n.run = j;
                        }
                    }
                };
            });
        });
    }

    initGraph() {
        if (this.graph) {
            this.graph.clean();
        }
        if (!this.graph || this.graph.direction !== this.direction) {
            this.graph = new WorkflowV2Graph(this.createForkJoinNodeComponent.bind(this), this.direction,
                GraphComponent.minScale,
                GraphComponent.maxScale);
        }
        this.navigationGraph = new NavigationGraph(this.nodes, this.direction);

        this.nodes.forEach(n => {
            let component: ComponentRef<WorkflowV2JobsGraphOrNodeOrMatrixComponent>;
            switch (n.type) {
                case GraphNodeType.Stage:
                    const r = this.createSubGraphComponent(n);
                    component = r;
                    this.graph.createNode(n.name, n, component, r.instance.graph.graph.graph().height + 2 * WorkflowV2Graph.marginSubGraph, r.instance.graph.graph.graph().width + 2 * WorkflowV2Graph.marginSubGraph);
                    break;
                case GraphNodeType.Matrix:
                    component = this.createJobMatrixComponent(n);
                    this.graph.createNode(n.name, n, component);
                    break;
                default:
                    component = this.createJobNodeComponent(n);
                    this.graph.createNode(n.name, n, component);
                    if (n.run) {
                        this.graph.setNodeStatus(n.name, n.run ? n.run.status : null);
                    }
                    break;
            }
        });

        this.nodes.forEach(n => {
            if (n.depends_on && n.depends_on.length > 0) {
                n.depends_on.forEach(d => {
                    this.graph.createEdge(`node-${d}`, `node-${n.name}`);
                });
            }
        });

        const element = this.svgContainer.element.nativeElement;

        this.graph.draw(element, true);

        this.resize();

        if (this.selectedNodeNavigationKey) {
            this.graph.selectNode(this.selectedNodeNavigationKey);
        }

        const runActive = (this._runJobs ?? []).filter(j => V2WorkflowRunJobStatusIsActive(j.status)).length > 0;
        this.graph.setRunActive(runActive);

        this._cd.markForCheck();
    }

    public resize() {
        if (!this.svgContainer?.element?.nativeElement?.offsetWidth || !this.svgContainer?.element?.nativeElement?.offsetHeight) {
            return;
        }
        this.graph.resize(this.svgContainer.element.nativeElement.offsetWidth, this.svgContainer.element.nativeElement.offsetHeight);
    }

    clickOrigin() {
        this.graph.center();
    }

    clickHook(type: string): void {
        this.onSelectHook.emit(type);
    }

    createJobNodeComponent(node: GraphNode): ComponentRef<GraphJobNodeComponent> {
        const componentRef = this.svgContainer.createComponent(GraphJobNodeComponent);
        componentRef.instance.node = node;
        componentRef.instance.actionCallback = this.onNodeAction.bind(this);
        componentRef.changeDetectorRef.detectChanges();
        return componentRef;
    }

    createJobMatrixComponent(node: GraphNode): ComponentRef<GraphMatrixNodeComponent> {
        const componentRef = this.svgContainer.createComponent(GraphMatrixNodeComponent);
        componentRef.instance.node = node;
        componentRef.instance.actionCallback = this.onNodeAction.bind(this);
        componentRef.changeDetectorRef.detectChanges();
        return componentRef;
    }

    createForkJoinNodeComponent(nodes: Array<GraphNode>, type: string): ComponentRef<GraphForkJoinNodeComponent> {
        const componentRef = this.svgContainer.createComponent(GraphForkJoinNodeComponent);
        componentRef.instance.nodes = nodes;
        componentRef.instance.type = type;
        componentRef.instance.actionCallback = this.onNodeAction.bind(this);
        componentRef.changeDetectorRef.detectChanges();
        return componentRef;
    }

    createSubGraphComponent(node: GraphNode): ComponentRef<GraphStageNodeComponent> {
        const componentRef = this.svgContainer.createComponent(GraphStageNodeComponent);
        componentRef.instance.graphNode = node;
        componentRef.instance.direction = this.direction;
        componentRef.instance.centerCallback = (n: GraphNode) => { this.graph.centerStage(`node-${node.name}`); };
        componentRef.instance.actionCallback = this.onNodeAction.bind(this);
        componentRef.changeDetectorRef.detectChanges();
        return componentRef;
    }

    onNodeAction(type: GraphNodeAction, n: GraphNode, options?: any) {
        switch (type) {
            case GraphNodeAction.Enter:
                this.graph.nodeMouseEvent(NodeMouseEvent.Enter, n.name, options);
                break;
            case GraphNodeAction.Out:
                this.graph.nodeMouseEvent(NodeMouseEvent.Out, n.name, options);
                break;
            case GraphNodeAction.Click:
            case GraphNodeAction.ClickGate:
                const baseKey = (n.job && n.job.stage) ? `${n.job.stage}-${n.name}` : n.name;
                this.selectedNodeNavigationKey = baseKey
                if (n.type === GraphNodeType.Matrix) { this.selectedNodeNavigationKey += '-' + options.jobMatrixKey; }
                this.graph.selectNode(this.selectedNodeNavigationKey);
                this.graph.centerNode(baseKey);
                if (options && options['jobRunID']) {
                    this.onSelectJobRun.emit(options['jobRunID']);
                } else if (options && options['gateName']) {
                    this.onSelectJobGate.emit(n.name);
                } else {
                    this.onSelectJob.emit(n.name);
                }
                break;
            case GraphNodeAction.ClickConfirmGate:
                this.onConfirmJobGate.emit(n.name);
                break;
            case GraphNodeAction.ClickRestart:
                this.onSelectJobRunRestart.emit(options['jobRunID']);
                break;
            case GraphNodeAction.ClickStop:
                this.onSelectJobRunStop.emit(options['jobRunID']);
                break;
        }
    }

    changeDirection(): void {
        this.direction = this.direction === GraphDirection.HORIZONTAL ? GraphDirection.VERTICAL : GraphDirection.HORIZONTAL;
        this.changeDisplay();
    }
}


