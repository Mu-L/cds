import { ChangeDetectionStrategy, ChangeDetectorRef, Component, Input, OnInit } from "@angular/core";
import { Router } from "@angular/router";
import { Concurrency, ProjectConcurrencyRuns } from "app/model/project.concurrency.model";
import { Project } from "app/model/project.model";
import { V2ProjectService } from "app/service/projectv2/project.service";
import { ErrorUtils } from "app/shared/error.utils";
import { NzMessageService } from "ng-zorro-antd/message";
import { lastValueFrom } from "rxjs";

@Component({
    selector: 'app-project-concurrencies',
    templateUrl: './concurrencies.html',
    styleUrls: ['./concurrencies.scss'],
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ProjectConcurrenciesComponent implements OnInit {
    @Input() project: Project;

    loading = { list: false, action: false };
    selectedConcurrency: Concurrency;
    concurrencies: Array<Concurrency> = [];
    concurrencyRuns: Array<ProjectConcurrencyRuns> = [];

    constructor(
        private _cd: ChangeDetectorRef,
        private _messageService: NzMessageService,
        private _v2ProjectService: V2ProjectService,
        private _router: Router
    ) { }

    ngOnInit(): void {
        this.load();
    }

    async load() {
        this.loading.list = true;
        this._cd.markForCheck();
        try {
            this.concurrencies = await lastValueFrom(this._v2ProjectService.getConcurrencies(this.project.key));
        } catch (e) {
            this._messageService.error(`Unable to load concurrencies: ${ErrorUtils.print(e)}`, { nzDuration: 2000 });
        }
        this.loading.list = false;
        this._cd.markForCheck();
    }

    async deleteConcurrency(c: Concurrency) {
        this.loading.action = true;
        this._cd.markForCheck();
        try {
            await lastValueFrom(this._v2ProjectService.deleteConcurrency(this.project.key, c.name))
            this.concurrencies = this.concurrencies.filter(s => s.name !== c.name);
            this._messageService.success(`Concurrency ${c.name} deleted`, { nzDuration: 2000 });
        } catch (e) {
            this._messageService.error(`Unable to delete concurrency: ${ErrorUtils.print(e)}`, { nzDuration: 2000 });
        }
        this.loading.action = false;
        this._cd.markForCheck();
    }

    async selectConcurrency(c: Concurrency) {
        this.selectedConcurrency = c;
       
        try {
            this.concurrencyRuns = await lastValueFrom(this._v2ProjectService.getConcurrencyRuns(this.project.key, this.selectedConcurrency.name))
        } catch (e) {
            this._messageService.error(`Unable to delete concurrency: ${ErrorUtils.print(e)}`, { nzDuration: 2000 });
        }
        this._cd.markForCheck;
    }

    unselectConcurrency(): void {
        delete this.selectedConcurrency;
    }

    sortByDate(a: ProjectConcurrencyRuns, b: ProjectConcurrencyRuns): number {
        return a.last_modified < b.last_modified ? -1 : 1;
    }
}