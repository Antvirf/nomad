/**
 * Copyright (c) HashiCorp, Inc.
 * SPDX-License-Identifier: MPL-2.0
 */

import { computed } from '@ember/object';

// An Ember.Computed property for summating all properties from a
// set of objects.
//
// ex. list: [ { foo: 1 }, { foo: 3 } ]
//     sum: sumAggregationProperty('list', 'foo') // 4
export default function sumAggregationProperty(listKey, propKey) {
  return computed(`${listKey}.@each.${propKey}`, function () {
    console.log('this, in context', this, listKey, this[listKey]);
    return (this[listKey] || this.get(listKey))
      .mapBy(propKey)
      .reduce((sum, count) => sum + count, 0);
  });
}
