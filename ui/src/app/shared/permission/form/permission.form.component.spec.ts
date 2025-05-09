import { HttpRequest, provideHttpClient, withInterceptorsFromDi } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { fakeAsync, flush, TestBed, tick } from '@angular/core/testing';
import { RouterTestingModule } from '@angular/router/testing';
import { TranslateLoader, TranslateModule, TranslateParser, TranslateService } from '@ngx-translate/core';
import { Group, GroupPermission } from 'app/model/group.model';
import { GroupService } from 'app/service/group/group.service';
import { SharedModule } from '../../shared.module';
import { PermissionEvent } from '../permission.event.model';
import { PermissionService } from '../permission.service';
import { PermissionFormComponent } from './permission.form.component';
import { BrowserAnimationsModule } from '@angular/platform-browser/animations';

describe('CDS: Permission From Component', () => {

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            declarations: [],
            providers: [
                GroupService,
                PermissionService,
                TranslateService,
                TranslateLoader,
                TranslateParser,
                provideHttpClient(withInterceptorsFromDi()),
                provideHttpClientTesting()
            ],
            imports: [
                SharedModule,
                BrowserAnimationsModule,
                TranslateModule.forRoot(),
                RouterTestingModule.withRoutes([])
            ]
        }).compileComponents();

    });


    it('should create new permission', fakeAsync(() => {
        const http = TestBed.get(HttpTestingController);

        let groupsMock = new Array<Group>();

        let groupMock = new Group();
        groupMock.id = 1;
        groupMock.name = 'grp1';
        groupMock.members = [];

        groupsMock.push(groupMock);

        // Create component
        let fixture = TestBed.createComponent(PermissionFormComponent);
        let component = fixture.debugElement.componentInstance;
        expect(component).toBeTruthy();

        http.expectOne(((req: HttpRequest<any>) => req.url === '/group')).flush(groupsMock);

        fixture.detectChanges();
        tick(50);

        let saveButton = fixture.debugElement.nativeElement.querySelector('button[disabled="true"][name="saveBtn"]');
        expect(saveButton).toBeTruthy();


        // Permission to add
        let gp = new GroupPermission();
        gp.group.name = 'grp1';
        gp.permission = 7;

        // Emulate typing
        fixture.componentInstance.newGroupPermission = gp;

        fixture.detectChanges();
        tick(50);

        // Click on create button
        spyOn(fixture.componentInstance.createGroupPermissionEvent, 'emit');
        saveButton = fixture.debugElement.nativeElement.querySelector('button[disabled="true"][name="saveBtn"]');
        expect(saveButton).toBeFalsy();
        saveButton = fixture.debugElement.nativeElement.querySelector('button[name="saveBtn"]');
        expect(saveButton).toBeTruthy();
        saveButton.click();

        // Check if creation event has been emitted
        expect(fixture.componentInstance.createGroupPermissionEvent.emit).toHaveBeenCalledWith(new PermissionEvent('add', gp));

        flush();
    }));
});

