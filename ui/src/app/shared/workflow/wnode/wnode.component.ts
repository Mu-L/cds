import { ChangeDetectionStrategy, ChangeDetectorRef, Component, Input, OnDestroy, OnInit, ViewChild } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { TranslateService } from '@ngx-translate/core';
import { Store } from '@ngxs/store';
import { PipelineStatus } from 'app/model/pipeline.model';
import { Project } from 'app/model/project.model';
import { WNode, WNodeJoin, WNodeTrigger, WNodeType, Workflow } from 'app/model/workflow.model';
import { WorkflowNodeRun } from 'app/model/workflow.run.model';
import { WorkflowCoreService } from 'app/service/workflow/workflow.core.service';
import { AutoUnsubscribe } from 'app/shared/decorator/autoUnsubscribe';
import { ToastService } from 'app/shared/toast/ToastService';
import { WorkflowWNodeMenuEditComponent } from 'app/shared/workflow/menu/edit-node/menu.edit.node.component';
import { WorkflowDeleteNodeComponent } from 'app/shared/workflow/modal/delete/workflow.node.delete.component';
import { WorkflowTriggerComponent } from 'app/shared/workflow/modal/node-add/workflow.trigger.component';
import { WorkflowNodeRunParamComponent } from 'app/shared/workflow/node/run/node.run.param.component';
import { ProjectState } from 'app/store/project.state';
import {
    AddJoinWorkflow,
    AddNodeTriggerWorkflow,
    OpenEditModal, SelectWorkflowNode,
    SelectWorkflowNodeRun,
    UpdateWorkflow
} from 'app/store/workflow.action';
import { WorkflowState } from 'app/store/workflow.state';
import { Subscription } from 'rxjs';
import { finalize, map } from 'rxjs/operators';
import { NzModalService } from 'ng-zorro-antd/modal';
import { WorkflowHookModalComponent } from 'app/shared/workflow/modal/hook-add/hook.modal.component';

@Component({
    selector: 'app-workflow-wnode',
    templateUrl: './workflow.node.html',
    styleUrls: ['./workflow.node.scss'],
    changeDetection: ChangeDetectionStrategy.OnPush
})
@AutoUnsubscribe()
export class WorkflowWNodeComponent implements OnInit, OnDestroy {

    // Data set by workflow graph
    @Input() node: WNode;
    @Input() workflow: Workflow;

    @ViewChild('menu')
    menu: WorkflowWNodeMenuEditComponent;

    project: Project;

    warnings = 0;
    loading: boolean;

    hasWorkflowRun: boolean;
    menuVisible: boolean = false;

    currentNodeRun: WorkflowNodeRun;
    nodeRunSub: Subscription;

    deleteNodeModalVisible: boolean = false;

    // Modal
    @ViewChild('workflowDeleteNode')
    workflowDeleteNode: WorkflowDeleteNodeComponent;

    constructor(
        private _activatedRoute: ActivatedRoute,
        private _router: Router,
        private _routerActivated: ActivatedRoute,
        private _store: Store,
        private _workflowCoreService: WorkflowCoreService,
        private _toast: ToastService,
        private _translate: TranslateService,
        private _cd: ChangeDetectorRef,
        private _modalService: NzModalService
    ) {
        this.project = this._store.selectSnapshot(ProjectState.projectSnapshot);
        let paramSnamp = this._routerActivated.snapshot.params;
        if (paramSnamp['number']) {
            this.hasWorkflowRun = true;
        }
    }

    ngOnDestroy(): void {} // Should be set to use @AutoUnsubscribe with AOT

    ngOnInit(): void {
        this.nodeRunSub = this._store.select(WorkflowState.nodeRunByNodeID)
            .pipe(map(filterFn => filterFn(this.node.id))).subscribe( nodeRun => {
            if (!nodeRun) {
                return;
            }
            if (this.currentNodeRun && this.currentNodeRun.id === nodeRun.id && this.currentNodeRun.status === nodeRun.status) {
                return;
            }
            this.currentNodeRun = nodeRun;
            if (this.currentNodeRun.status === PipelineStatus.SUCCESS) {
                this.computeWarnings();
            }
            this._cd.markForCheck();
        });
    }

        clickOnNode(): void {
        if (this.workflow.previewMode) {
            return;
        }
        if (!this.currentNodeRun) {
            this._store.dispatch(new SelectWorkflowNode({
                node: this.node
            }));
        } else {
            this._store.dispatch(new SelectWorkflowNodeRun({
                workflowNodeRun: this.currentNodeRun,
                node: this.node
            }));
        }

    }

    dblClickOnNode() {
        switch (this.node.type) {
            case WNodeType.PIPELINE:
                if (this.hasWorkflowRun && this.currentNodeRun) {
                    this._router.navigate([
                        'node', this.currentNodeRun.id
                    ], {
                            relativeTo: this._activatedRoute, queryParams: {
                                name: this.node.name,
                                node_id: this.node.id, node_ref: this.node.ref
                            }
                        });
                } else {
                    this._router.navigate([
                        '/project', this.project.key,
                        'pipeline', Workflow.getPipeline(this.workflow, this.node).name
                    ], { queryParams: { workflow: this.workflow.name, node_id: this.node.id, node_ref: this.node.ref } });
                }
                break;
            case WNodeType.OUTGOINGHOOK:
                if (this.hasWorkflowRun
                    && this.currentNodeRun
                    && this.node.outgoing_hook.config['target_project']
                    && this.node.outgoing_hook.config['target_workflow']
                    && this.currentNodeRun.callback) {
                    this._router.navigate([
                        '/project', this.node.outgoing_hook.config['target_project'].value,
                        'workflow', this.node.outgoing_hook.config['target_workflow'].value,
                        'run', this.currentNodeRun.callback.workflow_run_number
                    ], { queryParams: {} });
                }
                break;
        }
    }

    receivedEvent(e: string): void {
        this.menuVisible = false;
        switch (e) {
            case 'pipeline':
                this.openTriggerModal('pipeline', false);
                break;
            case 'parent':
                this.openTriggerModal('pipeline', true);
                break;
            case 'edit':
                this._store.dispatch(new OpenEditModal({
                    node: this.node,
                    hook: null
                }));
                break;
            case 'hook':
                this.openAddHookModal();
                break;
            case 'outgoinghook':
                this.openTriggerModal('outgoinghook', false);
                break;
            case 'fork':
                this.createFork();
                break;
            case 'join':
                this.createJoin();
                break;
            case 'join_link':
                this.linkJoin();
                break;
            case 'run':
                this.run();
                break;
            case 'delete':
                this.openDeleteNodeModal();
                break;
            case 'logs':
                this._router.navigate(['node', this.currentNodeRun.id], {
                    relativeTo: this._activatedRoute,
                    queryParams: { name: this.node.name }
                });
                break;
        }
        this._cd.markForCheck()
    }


    computeWarnings() {
        this.warnings = 0;
        if (!this.currentNodeRun || !this.currentNodeRun.stages) {
            return;
        }
        this.currentNodeRun.stages.forEach((stage) => {
            if (Array.isArray(stage.run_jobs)) {
                this.warnings += stage.run_jobs.reduce((fail, job) => {
                    if (!job.job || !Array.isArray(job.job.step_status)) {
                        return fail;
                    }
                    return fail + job.job.step_status.reduce((failStep, step) => {
                        if (step.status === PipelineStatus.FAIL) {
                            return failStep + 1;
                        }
                        return failStep;
                    }, 0);
                }, 0);
            }
        });
    }

    canEdit(): boolean {
        return this.workflow.permissions.writable;
    }

    openDeleteNodeModal(): void {
        if (!this.canEdit()) {
            return;
        }
        this._modalService.create({
            nzTitle: 'Delete ' + this.node.name,
            nzWidth: '900px',
            nzContent: WorkflowDeleteNodeComponent,
            nzData: {
                project: this.project,
                node: this.node,
                workflow: this.workflow
            }
        });
    }

    openTriggerModal(t: string, parent: boolean): void {
        if (this.canEdit()) {
            let title = 'Add trigger from' + this.node.name;
            if (parent) {
                title = 'Add parent to ' + this.node.name;
            }
            this._modalService.create({
                nzTitle: title,
                nzWidth: '1300px',
                nzContent: WorkflowTriggerComponent,
                nzData: {
                    project: this.project,
                    workflow: this.workflow,
                    source: this.node,
                    isParent: parent,
                    selectedType: t,
                }
            });
        }
    }

    openAddHookModal(): void {
        if (this.canEdit()) {
            this._modalService.create({
                nzTitle: 'Hook creation',
                nzWidth: '900px',
                nzContent: WorkflowHookModalComponent,
                nzData: {
                    project: this.project,
                    workflow: this.workflow,
                    node: this.node
                }
            });
        }
    }

    createFork(): void {
        let editMode = this._store.selectSnapshot(WorkflowState.current).editMode;
        let n: WNode;
        if (editMode) {
            n = Workflow.getNodeByRef(this.node.ref, this.workflow);
        } else {
            n = Workflow.getNodeByID(this.node.id, this.workflow);
        }
        let fork = new WNode();
        fork.ref = new Date().getTime().toString();
        fork.type = WNodeType.FORK;
        let t = new WNodeTrigger();
        t.child_node = fork;
        t.parent_node_id = n.id;
        t.parent_node_name = n.ref;

        this._store.dispatch(new AddNodeTriggerWorkflow({
            projectKey: this.project.key,
            workflowName: this.workflow.name,
            parentId: this.node.id,
            trigger: t
        }));
    }

    createJoin(): void {
        let join = new WNode();
        join.ref = new Date().getTime().toString();
        join.type = WNodeType.JOIN;
        join.parents = new Array<WNodeJoin>();
        let p = new WNodeJoin();
        p.parent_id = this.node.id;
        p.parent_name = this.node.ref;
        join.parents.push(p);

        this._store.dispatch(new AddJoinWorkflow({
            projectKey: this.project.key,
            workflowName: this.workflow.name,
            join
        }));
    }

    linkJoin(): void {
        if (!this.canEdit()) {
            return;
        }
        this._workflowCoreService.linkJoinEvent(this.node);
    }

    run(): void {
        this._modalService.create({
            nzWidth: '900px',
            nzTitle: 'Run workflow',
            nzContent: WorkflowNodeRunParamComponent,
        });
    }
}
