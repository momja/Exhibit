// One application per document. ES modules run once, including when multiple
// page scripts import this entry point. Register new shared controllers here.
import { Application } from '../stimulus/stimulus.js';
import ActionMenuController from './action-menu-controller.js';
import ModalController from './modal-controller.js';
import RelayHostController from './relay-host-controller.js';
import ConnectionsController from './connections-controller.js';
import UnsavedChangesController from './unsaved-changes-controller.js';

import ClerkController from './clerk-controller.js';
import AccountDeletionController from './account-deletion-controller.js';

export const application = Application.start();
application.register('action-menu', ActionMenuController);
application.register('modal', ModalController);
application.register('unsaved-changes', UnsavedChangesController);

application.register('clerk', ClerkController);
application.register('account-deletion', AccountDeletionController);

import FilterSheetController from './filter-sheet-controller.js';
application.register('filter-sheet', FilterSheetController);

import FloatingHeaderController from './floating-header-controller.js';
application.register('floating-header', FloatingHeaderController);

import GalleryController from './gallery-controller.js';
import GallerySpaceController from './gallery-space-controller.js';
application.register('gallery', GalleryController);
application.register('gallery-space', GallerySpaceController);

import SpaceTabsController from './space-tabs-controller.js';
application.register('space-tabs', SpaceTabsController);
application.register('relay-host', RelayHostController);
application.register('connections', ConnectionsController);
