/**
 * Copyright (c) HashiCorp, Inc.
 * SPDX-License-Identifier: MPL-2.0
 */

import 'core-js';
import Application from 'nomad-ui/app';
import config from 'nomad-ui/config/environment';
import * as QUnit from 'qunit';
import { setApplication } from '@ember/test-helpers';
import start from 'ember-exam/test-support/start';
import { setup } from 'qunit-dom';
import './helpers/flash-message';

setApplication(Application.create(config.APP));

setup(QUnit.assert);

start();
